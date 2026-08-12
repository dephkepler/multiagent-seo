package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/leadstats"
)

type LeadStatsRepository struct {
	db *pgxpool.Pool
}

func NewLeadStatsRepository(db *pgxpool.Pool) *LeadStatsRepository {
	return &LeadStatsRepository{db: db}
}

// Totals counts leads by received_at and consultations by created_at —
// deliberately not the same column, since a consultation's created_at is
// "when it was booked" (what happened this period) while its scheduled_at
// is a future appointment date; booked-in-period is the useful number here.
func (r *LeadStatsRepository) Totals(ctx context.Context, from, to time.Time) (leadstats.Totals, error) {
	var t leadstats.Totals

	const leadsQ = `
		SELECT count(*), count(DISTINCT client_id) FILTER (WHERE client_id IS NOT NULL)
		FROM leads WHERE received_at BETWEEN @from AND @to`
	if err := r.db.QueryRow(ctx, leadsQ, pgx.NamedArgs{"from": from, "to": to}).
		Scan(&t.Leads, &t.Clients); err != nil {
		return t, fmt.Errorf("totals leads: %w", err)
	}

	var revenue *float64
	const consultQ = `
		SELECT count(*), sum(price) FILTER (WHERE price > 0)
		FROM consultations WHERE created_at BETWEEN @from AND @to`
	if err := r.db.QueryRow(ctx, consultQ, pgx.NamedArgs{"from": from, "to": to}).
		Scan(&t.Consultations, &revenue); err != nil {
		return t, fmt.Errorf("totals consultations: %w", err)
	}
	if revenue != nil {
		t.Revenue = *revenue
	}

	const paidQ = `SELECT count(*) FROM consultations WHERE created_at BETWEEN @from AND @to AND price > 0`
	var paidCount int64
	if err := r.db.QueryRow(ctx, paidQ, pgx.NamedArgs{"from": from, "to": to}).Scan(&paidCount); err != nil {
		return t, fmt.Errorf("totals paid count: %w", err)
	}
	if paidCount > 0 {
		t.AvgTicket = t.Revenue / float64(paidCount)
	}
	return t, nil
}

// Trend buckets leads (by received_at) and consultations (by created_at)
// into the same day/month buckets and full-outer-joins them, so a bucket
// with only one of the two still shows up with 0 on the other side.
func (r *LeadStatsRepository) Trend(ctx context.Context, from, to time.Time, groupBy string) ([]leadstats.Bucket, error) {
	trunc := "day"
	if groupBy == "month" {
		trunc = "month"
	}

	q := fmt.Sprintf(`
		WITH l AS (
			SELECT date_trunc('%s', received_at) AS bucket, count(*) AS leads
			FROM leads WHERE received_at BETWEEN @from AND @to
			GROUP BY 1
		), c AS (
			SELECT date_trunc('%s', created_at) AS bucket, count(*) AS consultations
			FROM consultations WHERE created_at BETWEEN @from AND @to
			GROUP BY 1
		)
		SELECT coalesce(l.bucket, c.bucket) AS bucket, coalesce(l.leads, 0), coalesce(c.consultations, 0)
		FROM l FULL OUTER JOIN c ON l.bucket = c.bucket
		ORDER BY 1`, trunc, trunc)
	// trunc is one of two hardcoded literals picked above, never caller input —
	// safe to interpolate, date_trunc's unit can't be a bind parameter in pgx.

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"from": from, "to": to})
	if err != nil {
		return nil, fmt.Errorf("trend: %w", err)
	}
	defer rows.Close()

	var out []leadstats.Bucket
	for rows.Next() {
		var b leadstats.Bucket
		var bucketAt time.Time
		if err := rows.Scan(&bucketAt, &b.Leads, &b.Consultations); err != nil {
			return nil, fmt.Errorf("trend: scan: %w", err)
		}
		if trunc == "month" {
			b.Bucket = bucketAt.Format("2006-01")
		} else {
			b.Bucket = bucketAt.Format("2006-01-02")
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trend: %w", err)
	}
	return out, nil
}

func (r *LeadStatsRepository) ByPage(ctx context.Context, from, to time.Time) ([]leadstats.Count, error) {
	const q = `
		SELECT page, count(*) FROM leads
		WHERE received_at BETWEEN @from AND @to
		GROUP BY page ORDER BY count(*) DESC`
	return r.queryCounts(ctx, q, from, to)
}

func (r *LeadStatsRepository) ByCreator(ctx context.Context, from, to time.Time) ([]leadstats.Count, error) {
	const q = `
		SELECT created_by, count(*) FROM consultations
		WHERE created_at BETWEEN @from AND @to
		GROUP BY created_by ORDER BY count(*) DESC`
	return r.queryCounts(ctx, q, from, to)
}

// ByConsultationStatus feeds the "what happened to booked consultations"
// panel — the funnel-outcome view (scheduled/completed/cancelled/no_show).
// Every consultation always has a status (DB default 'scheduled'), so this
// slice's counts sum to exactly Totals.Consultations for the same range —
// no separate denominator needed to turn a count into a percentage.
func (r *LeadStatsRepository) ByConsultationStatus(ctx context.Context, from, to time.Time) ([]leadstats.Count, error) {
	const q = `
		SELECT status, count(*) FROM consultations
		WHERE created_at BETWEEN @from AND @to
		GROUP BY status ORDER BY count(*) DESC`
	return r.queryCounts(ctx, q, from, to)
}

func (r *LeadStatsRepository) queryCounts(ctx context.Context, q string, from, to time.Time) ([]leadstats.Count, error) {
	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"from": from, "to": to})
	if err != nil {
		return nil, fmt.Errorf("query counts: %w", err)
	}
	defer rows.Close()

	var out []leadstats.Count
	for rows.Next() {
		var c leadstats.Count
		if err := rows.Scan(&c.Key, &c.Count); err != nil {
			return nil, fmt.Errorf("query counts: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query counts: %w", err)
	}
	return out, nil
}
