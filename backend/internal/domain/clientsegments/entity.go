// Package clientsegments answers "where is this client in the funnel" —
// обращение → консультація → дело — straight from leads/consultations/cases.
// Nothing is stored or assigned by hand: Derive is a pure function over
// facts already sitting in those tables, so a client's segment can never
// drift out of sync with reality the way a manually-maintained tag would.
package clientsegments

import "time"

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
)

// NoShowRiskThreshold — один сорвавшийся визит бывает у кого угодно, два и
// больше уже похоже на паттерн, а не на случайность.
const NoShowRiskThreshold = 2

// Activity is the raw per-client facts the repository reads straight off
// leads/consultations/cases. Kept separate from ClientSegment so Derive's
// classification rules are plain Go, testable without a database.
type Activity struct {
	ClientID       string
	Name           string
	Phone          string
	CompletedCount int
	ScheduledCount int
	LostCount      int // cancelled + no_show
	ConsultCount   int
	CaseCount      int
	CaseFee        float64
	CasePaid       float64
	LastActivity   time.Time
}

type ClientSegment struct {
	ClientID     string
	Name         string
	Phone        string
	Segment      string
	Tags         []string
	LastActivity time.Time
	CaseCount    int
	CaseFee      float64
	CasePaid     float64
}

// Derive classifies one client's funnel position from their raw activity.
func Derive(a Activity) ClientSegment {
	cs := ClientSegment{
		ClientID:     a.ClientID,
		Name:         a.Name,
		Phone:        a.Phone,
		LastActivity: a.LastActivity,
		CaseCount:    a.CaseCount,
		CaseFee:      a.CaseFee,
		CasePaid:     a.CasePaid,
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
	return cs
}
