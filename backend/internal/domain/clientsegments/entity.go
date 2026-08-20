// Package clientsegments derives each client's funnel segment from
// leads/consultations/cases; SegmentOverride is the one exception where
// staff can pin it by hand instead.
package clientsegments

import (
	"errors"
	"time"
)

var ErrInvalidSegment = errors.New("clientsegments: invalid segment")
var ErrNotFound = errors.New("clientsegments: client not found")
var ErrInvalidSort = errors.New("clientsegments: invalid sort")

// empty or over ManualTagMaxLen.
var ErrInvalidManualTag = errors.New("clientsegments: invalid manual tag")

// AddTag when label isn't in client_tag_defs — closed vocabulary, not typo-tolerant.
var ErrUnknownTag = errors.New("clientsegments: unknown tag")

// A CreateTagDef/UpdateTagDef label collision — labels are the primary key.
var ErrTagDefExists = errors.New("clientsegments: tag definition already exists")

// UpdateTagDef/DeleteTagDef against a label that isn't in the vocabulary.
var ErrTagDefNotFound = errors.New("clientsegments: tag definition not found")

// keeps a manual tag (or a tag def's label/category) short — notes have
// their own field (see clientdetail) for anything longer.
const ManualTagMaxLen = 40

// DefaultTagCategory is what a tag def gets when nothing more specific
// applies — the vocabulary's one catch-all group.
const DefaultTagCategory = "Інше"

// TagDef is one entry in the manual-tag vocabulary — see client_tag_defs.
// AddTag only accepts a Label already defined here (enforced by a DB FK):
// the whole point is a curated, stable list, not whatever text a staff
// member happens to type. Category groups defs into the two-level
// (category → tag) structure the picker and management UI both show —
// picked freely by whoever manages the vocabulary, not a fixed enum.
type TagDef struct {
	Label     string
	Category  string
	CreatedAt time.Time
}

const (
	SegmentLead      = "lead"
	SegmentBooked    = "booked"
	SegmentConsulted = "consulted"
	SegmentClient    = "client"
	SegmentRepeat    = "repeat"
	SegmentLost      = "lost"
)

// Tag is a non-exclusive flag layered on top of Segment.
const (
	TagDebtor     = "debtor"
	TagNoShowRisk = "no_show_risk"
	TagHighValue  = "high_value"
	TagDormant    = "dormant"
)

// 2+ no-shows reads as a pattern, not a one-off.
const NoShowRiskThreshold = 2

// data-driven cutoff, not round — see 2026-08-17 LTV snapshot.
const HighValueThreshold = 5000

// 90d idle — historical import means most clients read dormant for now.
const DormantThreshold = 90 * 24 * time.Hour

func IsSegment(s string) bool {
	switch s {
	case SegmentLead, SegmentBooked, SegmentConsulted, SegmentClient, SegmentRepeat, SegmentLost:
		return true
	default:
		return false
	}
}

// always descending — both only make sense high-to-low.
const (
	SortActivity = "activity"
	SortLTV      = "ltv"
)

// zero field = no filter, except Limit (defaults to 25).
type ListFilter struct {
	ClientID string
	Segment  string
	Tag      string
	Search   string
	// Sort is SortActivity or SortLTV; "" defaults to SortActivity.
	Sort   string
	Limit  int
	Offset int
}

type ClientList struct {
	Items []ClientSegment
	Total int
	// SegmentCounts ignores ListFilter — always all clients, for the pill counts.
	SegmentCounts map[string]int
}

type Activity struct {
	ClientID       string
	Name           string
	Phone          string
	CompletedCount int
	ScheduledCount int
	LostCount      int // cancelled + no_show
	ConsultCount   int
	ConsultRevenue float64 // completed, priced — same filter as leadstats' "Earned"
	CaseCount      int
	CaseFee        float64
	CasePaid       float64
	LastActivity   time.Time
	// nil means no manual pin; non-nil wins over the computed segment.
	SegmentOverride *string
	// staff-added (client_tags table); unlike Tag*, never recomputed by Derive.
	ManualTags []string
}

type ClientSegment struct {
	ClientID     string
	Name         string
	Phone        string
	Segment      string
	Overridden   bool     // true when Segment came from SegmentOverride, not derived
	Tags         []string // auto-computed only — TagDebtor/TagNoShowRisk/TagHighValue/TagDormant, see Derive
	ManualTags   []string // staff-added, free text — see AddTag/RemoveTag, not touched by Derive
	LastActivity time.Time
	CaseCount    int
	CaseFee      float64
	CasePaid     float64
	// must stay in sync with clientdetail.Detail.RevenueTotal
	LTV float64
}

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
		ManualTags:   a.ManualTags,
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
		// completed and scheduled are already ruled out above, so this is cancelled/no_show.
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

	// override applies last, after tags, so tags stay tied to the real numbers.
	if a.SegmentOverride != nil {
		cs.Segment = *a.SegmentOverride
		cs.Overridden = true
	}
	return cs
}
