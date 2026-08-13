// Package leadstats aggregates the leads/clients/consultations tables into
// the numbers the admin dashboard shows — counts, revenue, breakdown by
// source page and by who booked the consultation. Read-only: nothing here
// writes to those tables (see webleads and consultations for that).
package leadstats

import "time"

// Revenue is split by outcome, not just summed — a cancelled consultation's
// price is not money the business has, so lumping it into one "Revenue"
// number (as the first version of this package did) overstates it. Booked
// is the full potential (every priced consultation regardless of status),
// Earned is only completed ones, Lost is what cancelled/no_show ones would
// have been worth.
type Totals struct {
	Leads         int64
	Clients       int64
	Consultations int64
	RevenueBooked float64
	RevenueEarned float64
	RevenueLost   float64
	AvgTicket     float64 // average price of a *completed* consultation

	// Cases (дела) are where most of the actual money is — a case's fee
	// (contract amount) runs 1,000–15,000+ ₴, an order of magnitude above a
	// consultation's 500–800 ₴. See doc/abalisbotlead for the numbers this
	// was built to answer.
	CasesInProgress   int64
	CasesCompleted    int64
	CaseFeeContracted float64 // sum of every case's agreed fee
	CasePaid          float64 // sum of what's actually been received so far
	CaseOwed          float64 // sum of (fee - paid) across cases still owing — the collections number

	// Site traffic (GA4) — optional, both stay 0 if CF_GA4_PROPERTY_ID isn't
	// configured. Sessions is every visit regardless of source, Organic is
	// just organic-search sessions — the old spreadsheet's "Сайт,
	// посетители" / "Сео" columns.
	SiteSessions    int64
	OrganicSessions int64
}

// Bucket is one point on the trend chart — a day or a month, per GroupBy.
type Bucket struct {
	Bucket          string
	Leads           int64
	Consultations   int64
	RevenueEarned   float64
	SiteSessions    int64 // 0 if GA4 isn't configured, or this bucket predates it
	OrganicSessions int64
}

// TrafficBucket is what a TrafficSource returns per period — merged into
// Bucket by matching Bucket keys (same "2006-01-02"/"2006-01" format),
// not carried as its own parallel list.
type TrafficBucket struct {
	Bucket          string
	Sessions        int64
	OrganicSessions int64
}

// Count is a generic "this key, this many" row — used for the by-page
// breakdown (and by-status, which has no revenue dimension of its own).
type Count struct {
	Key   string
	Count int64
}

// CreatorRevenue is who-booked-it broken down by both load and money — a
// plain Count would only show who books the most, not who actually closes
// revenue (a big booker with a high cancel rate can earn less than someone
// who books fewer but sees them through).
type CreatorRevenue struct {
	Key           string
	Bookings      int64
	RevenueEarned float64
}

// CategoryRevenue answers "which direction (practice area) actually makes
// money" — Contracted is the full deal size, Paid what's actually been
// collected so far, same Booked-vs-Earned distinction as consultations.
type CategoryRevenue struct {
	Key        string
	Cases      int64
	Contracted float64
	Paid       float64
}

type Stats struct {
	From       time.Time
	To         time.Time
	GroupBy    string
	Totals     Totals
	Trend      []Bucket
	ByPage     []Count
	ByCreator  []CreatorRevenue
	ByStatus   []Count
	ByCategory []CategoryRevenue
}
