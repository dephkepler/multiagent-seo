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
}

// Bucket is one point on the trend chart — a day or a month, per GroupBy.
type Bucket struct {
	Bucket        string
	Leads         int64
	Consultations int64
	RevenueEarned float64
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

type Stats struct {
	From      time.Time
	To        time.Time
	GroupBy   string
	Totals    Totals
	Trend     []Bucket
	ByPage    []Count
	ByCreator []CreatorRevenue
	ByStatus  []Count
}
