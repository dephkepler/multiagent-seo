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
	//
	// CasesCompleted/CaseFeeContracted/CasePaid are scoped to cases opened
	// in [From, To] — "what happened this period", same as the
	// consultation revenue fields above. CasesInProgress/CaseOwed are NOT:
	// they're a live snapshot across every case regardless of when it was
	// opened, because "how many cases are active" and "who owes us money"
	// describe the business's current state — a case opened last month and
	// still unpaid is real debt today, and a short date range picked on
	// this dashboard shouldn't make it disappear from the number.
	CasesInProgress   int64   // live count, ignores [From, To]
	CasesCompleted    int64   // opened in [From, To]
	CaseFeeContracted float64 // sum of agreed fee, cases opened in [From, To]
	CasePaid          float64 // sum of what's been received so far, cases opened in [From, To]
	CaseOwed          float64 // live sum of (fee - paid) across every case still owing, ignores [From, To] — the collections number

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

// Count is a generic "this key, this many" row — used for by-status, which
// has no revenue dimension of its own.
type Count struct {
	Key   string
	Count int64
}

// SourceValue answers "which source brings valuable leads, not just many
// leads" — same first-touch cohort principle as Funnel: each client is
// attributed to the page of their FIRST-ever lead, then followed to revenue
// with no date bound of its own, regardless of when the cohort window ends.
type SourceValue struct {
	Key           string // leads.page of the client's first lead ("" — no page, e.g. an email lead)
	Leads         int64  // distinct clients whose first lead fell in the period, attributed to this source
	ConsultedEver int64
	CasedEver     int64
	RevenueEarned float64 // consultation revenue (completed, priced), all-time
	CasePaid      float64 // case payments received, all-time
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

// Funnel is a proper cohort conversion, not a period ratio — the naive
// "consultations this period / leads this period" mixes different leads in
// numerator and denominator (a lead from day 25 usually converts weeks
// later, outside a short window). Instead: take every client whose first
// lead fell in [From, To] (the cohort), then check — with no upper bound on
// when — whether they ever got a consultation, and ever got a case. This is
// how cohort retention/conversion is measured in any real CRM (HubSpot,
// Amplitude, ...), not something invented for this project.
type Funnel struct {
	CohortLeads   int64 // distinct clients whose first lead fell in the period
	ConsultedEver int64 // of those, how many have ANY consultation, any time
	CasedEver     int64 // of those, how many have ANY case, any time

	// Time-to-convert — only over clients who actually converted, so a slow
	// trickle of eventual conversions doesn't get diluted by the ones who
	// never will. 0 when nobody in the cohort converted yet.
	AvgDaysToConsult float64
	AvgDaysToCase    float64
}

type Stats struct {
	From       time.Time
	To         time.Time
	GroupBy    string
	Totals     Totals
	Trend      []Bucket
	BySource   []SourceValue
	ByCreator  []CreatorRevenue
	ByStatus   []Count
	ByCategory []CategoryRevenue
	ByAdvocate []CategoryRevenue // case revenue by advocate_name — see Repository.ByCaseAdvocate
	Funnel     Funnel
	// ByWeekday is leads grouped by ISO day-of-week ("1" Monday .. "7"
	// Sunday) — deliberately no by-hour counterpart: 772 of ~800 historical
	// leads all carry received_at's clock time as a flat 12:00 (the 55k
	// import only had a date column, no time, and defaulted it), so an
	// hour breakdown would mostly be showing that artifact back. The date
	// itself is real even for imported rows, so weekday is fine.
	ByWeekday []Count
}
