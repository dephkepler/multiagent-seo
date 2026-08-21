package leadstats

import (
	"context"
	"time"
)

// Repository reads aggregates only — from and to are an inclusive range,
// already resolved to timestamps by the caller (end-of-day for "to").
type Repository interface {
	Totals(ctx context.Context, from, to time.Time) (Totals, error)
	Trend(ctx context.Context, from, to time.Time, groupBy string) ([]Bucket, error)
	// BySource is a cohort conversion by leads.page — see the SourceValue
	// doc comment for why this replaced a plain lead-count-by-page.
	BySource(ctx context.Context, from, to time.Time) ([]SourceValue, error)
	ByCreator(ctx context.Context, from, to time.Time) ([]CreatorRevenue, error)
	ByConsultationStatus(ctx context.Context, from, to time.Time) ([]Count, error)
	ByCaseCategory(ctx context.Context, from, to time.Time) ([]CategoryRevenue, error)
	// ByCaseAdvocate is case revenue (not consultation bookings) grouped by
	// advocate_name — ByCreator above only covers who *booked* a
	// consultation, not who actually closed the real money.
	ByCaseAdvocate(ctx context.Context, from, to time.Time) ([]CategoryRevenue, error)
	// Funnel is a cohort conversion — see the Funnel doc comment for why
	// this isn't just Totals.Consultations / Totals.Leads.
	Funnel(ctx context.Context, from, to time.Time) (Funnel, error)
	// ByWeekday is leads by ISO day-of-week, Key "1".."7" — see the
	// Stats.ByWeekday doc for why there's no by-hour equivalent.
	ByWeekday(ctx context.Context, from, to time.Time) ([]Count, error)
}

// TrafficSource is GA4 — optional (the service works fine with it nil, same
// no-op pattern as the Sheets sink elsewhere in the codebase).
type TrafficSource interface {
	SessionsByPeriod(ctx context.Context, from, to time.Time, groupBy string) ([]TrafficBucket, error)
	// Audience reports GA4's visitor demographics/geography for the
	// period — see the Audience doc comment.
	Audience(ctx context.Context, from, to time.Time) (Audience, error)
}
