// Package leadstats aggregates the leads/clients/consultations tables into
// the numbers the admin dashboard shows — counts, revenue, breakdown by
// source page and by who booked the consultation. Read-only: nothing here
// writes to those tables (see webleads and consultations for that).
package leadstats

import "time"

type Totals struct {
	Leads         int64
	Clients       int64
	Consultations int64
	Revenue       float64
	AvgTicket     float64
}

// Bucket is one point on the trend chart — a day or a month, per GroupBy.
type Bucket struct {
	Bucket        string
	Leads         int64
	Consultations int64
}

// Count is a generic "this key, this many" row — used for both the
// by-page and by-creator breakdowns.
type Count struct {
	Key   string
	Count int64
}

type Stats struct {
	From      time.Time
	To        time.Time
	GroupBy   string
	Totals    Totals
	Trend     []Bucket
	ByPage    []Count
	ByCreator []Count
}
