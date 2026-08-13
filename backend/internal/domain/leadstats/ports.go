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
	ByPage(ctx context.Context, from, to time.Time) ([]Count, error)
	ByCreator(ctx context.Context, from, to time.Time) ([]CreatorRevenue, error)
	ByConsultationStatus(ctx context.Context, from, to time.Time) ([]Count, error)
	ByCaseCategory(ctx context.Context, from, to time.Time) ([]CategoryRevenue, error)
}

// TrafficSource is GA4 — optional (the service works fine with it nil, same
// no-op pattern as the Sheets sink elsewhere in the codebase).
type TrafficSource interface {
	SessionsByPeriod(ctx context.Context, from, to time.Time, groupBy string) ([]TrafficBucket, error)
}
