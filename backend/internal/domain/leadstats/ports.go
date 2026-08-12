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
}
