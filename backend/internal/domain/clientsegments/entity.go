// Package clientsegments answers "where is this client in the funnel" —
// обращение → консультація → дело — straight from leads/consultations/cases.
// By default nothing is stored or assigned by hand: Derive is a pure
// function over facts already sitting in those tables, so a client's
// segment can't drift out of sync with reality the way a manually-
// maintained tag would. The one deliberate exception is SegmentOverride —
// staff can pin a segment by hand for the cases the data genuinely can't
// capture (a client who moved to another advocate outside this system
// entirely — see ABL 024); Derive still runs underneath, the override just
// wins at the end, and ClientSegment.Overridden says so.
package clientsegments

import (
	"errors"
	"time"
)

// ErrInvalidSegment is returned when a manual override names a segment
// outside the known Segment* set (see IsSegment).
var ErrInvalidSegment = errors.New("clientsegments: invalid segment")

// ErrNotFound is returned when an override targets a client id that
// doesn't exist.
var ErrNotFound = errors.New("clientsegments: client not found")

// Segment values, one per client — a straight-line funnel, checked in
// Derive in the order a client actually moves through it (a case beats a
// cancelled consultation history, so someone with both reads as Client,
// not Lost).
const (
	SegmentLead      = "lead"      // заявка есть, ни одной консультации
	SegmentBooked    = "booked"    // есть scheduled, ни одной completed
	SegmentConsulted = "consulted" // была completed, дела нет — самый денежный сегмент, есть кого дожимать
	SegmentClient    = "client"    // есть хотя бы одно дело
	SegmentRepeat    = "repeat"    // 2+ дела
	SegmentLost      = "lost"      // все консультации cancelled/no_show, дела нет
)

// Tag is a non-exclusive flag layered on top of Segment.
const (
	TagDebtor     = "debtor"       // есть дело с paid_amount < fee
	TagNoShowRisk = "no_show_risk" // NoShowRiskThreshold+ отменённых/неявок подряд
	TagHighValue  = "high_value"   // LTV >= HighValueThreshold
	TagDormant    = "dormant"      // ни одного касания за DormantThreshold
)

// NoShowRiskThreshold — один сорвавшийся визит бывает у кого угодно, два и
// больше уже похоже на паттерн, а не на случайность.
const NoShowRiskThreshold = 2

// HighValueThreshold — граница по факту в проде, не круглое число с потолка:
// на срезе от 2026-08-17 (658 клиентов, 76 с LTV > 0) здесь настоящий разрыв
// в хвосте распределения — 8 клиентов ≥ 5000₴ (это же p90 среди платящих),
// следующий вниз уже 2550₴. Пересчитать при повторном ревью порога — запрос
// см. в чате.
const HighValueThreshold = 5000

// DormantThreshold — три месяца без единого касания в живом деле или заявке.
// Меньше — нормальный разрыв между визитами, не сигнал; дольше — момент для
// повторного контакта уже упущен. На срезе 2026-08-17 почти вся база старше
// этого порога (единоразовый импорт исторических лидов 2024–2025, живая
// работа началась только в этом месяце) — тег в моменте не избирательный, но
// корректно описывает, что база холодная, и станет полезным фильтром по мере
// накопления свежей активности.
const DormantThreshold = 90 * 24 * time.Hour

// IsSegment reports whether s is one of the known Segment* values — used to
// validate a manual override before it ever reaches the database.
func IsSegment(s string) bool {
	switch s {
	case SegmentLead, SegmentBooked, SegmentConsulted, SegmentClient, SegmentRepeat, SegmentLost:
		return true
	default:
		return false
	}
}

// Activity is the raw per-client facts the repository reads straight off
// leads/consultations/cases, plus whatever manual override is on file for
// them. Kept separate from ClientSegment so Derive's classification rules
// are plain Go, testable without a database.
type Activity struct {
	ClientID       string
	Name           string
	Phone          string
	CompletedCount int
	ScheduledCount int
	LostCount      int // cancelled + no_show
	ConsultCount   int
	ConsultRevenue float64 // completed, priced consultations — same filter as leadstats' "Заработано"
	CaseCount      int
	CaseFee        float64
	CasePaid       float64
	LastActivity   time.Time
	// SegmentOverride is nil when staff hasn't pinned a segment for this
	// client — Derive computes one as usual. Non-nil wins over the
	// computed value; see the package doc.
	SegmentOverride *string
}

type ClientSegment struct {
	ClientID     string
	Name         string
	Phone        string
	Segment      string
	Overridden   bool // true when Segment came from SegmentOverride, not the funnel rules below
	Tags         []string
	LastActivity time.Time
	CaseCount    int
	CaseFee      float64
	CasePaid     float64
	// LTV is lifetime value — money actually collected, all-time: completed
	// consultations plus case payments. Same "earned, not booked" shape as
	// clientdetail.Detail.RevenueTotal (kept in sync with it deliberately —
	// the client-list ranking and the single-client card must agree on what
	// a client is worth).
	LTV float64
}

// Derive classifies one client's funnel position from their raw activity.
// now is the reference point for DormantThreshold — passed in rather than
// read from the clock here so the classification stays a pure, testable
// function of its inputs.
func Derive(a Activity, now time.Time) ClientSegment {
	cs := ClientSegment{
		ClientID:     a.ClientID,
		Name:         a.Name,
		Phone:        a.Phone,
		LastActivity: a.LastActivity,
		CaseCount:    a.CaseCount,
		CaseFee:      a.CaseFee,
		CasePaid:     a.CasePaid,
		LTV:          a.ConsultRevenue + a.CasePaid,
	}

	switch {
	case a.CaseCount >= 2:
		cs.Segment = SegmentRepeat
	case a.CaseCount == 1:
		cs.Segment = SegmentClient
	case a.CompletedCount > 0:
		cs.Segment = SegmentConsulted
	case a.ScheduledCount > 0:
		cs.Segment = SegmentBooked
	case a.ConsultCount > 0:
		// completed == 0 and scheduled == 0 already ruled out above, so any
		// consultation left at this point is cancelled or no_show.
		cs.Segment = SegmentLost
	default:
		cs.Segment = SegmentLead
	}

	if a.CaseFee > a.CasePaid {
		cs.Tags = append(cs.Tags, TagDebtor)
	}
	if a.LostCount >= NoShowRiskThreshold {
		cs.Tags = append(cs.Tags, TagNoShowRisk)
	}
	if cs.LTV >= HighValueThreshold {
		cs.Tags = append(cs.Tags, TagHighValue)
	}
	if now.Sub(a.LastActivity) >= DormantThreshold {
		cs.Tags = append(cs.Tags, TagDormant)
	}

	// Override wins last, after tags — tags stay tied to the real numbers
	// even when staff has pinned the segment itself by hand.
	if a.SegmentOverride != nil {
		cs.Segment = *a.SegmentOverride
		cs.Overridden = true
	}
	return cs
}
