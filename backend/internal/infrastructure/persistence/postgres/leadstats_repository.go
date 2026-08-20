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
//
// Revenue is split by status in one pass (FILTER, not three separate
// queries) — Booked is every priced consultation regardless of outcome,
// Earned only completed ones, Lost what cancelled/no_show ones would have
// been worth. AvgTicket is the average price of a *completed* consultation
// (not of everything booked) — the useful "typical sale size" number.
func (r *LeadStatsRepository) Totals(ctx context.Context, from, to time.Time) (leadstats.Totals, error) {
	var t leadstats.Totals

	const leadsQ = `
		SELECT count(*), count(DISTINCT client_id) FILTER (WHERE client_id IS NOT NULL)
		FROM leads WHERE received_at BETWEEN @from AND @to`
	if err := r.db.QueryRow(ctx, leadsQ, pgx.NamedArgs{"from": from, "to": to}).
		Scan(&t.Leads, &t.Clients); err != nil {
		return t, fmt.Errorf("totals leads: %w", err)
	}

	const consultQ = `
		SELECT
			count(*),
			coalesce(sum(price) FILTER (WHERE price > 0), 0),
			coalesce(sum(price) FILTER (WHERE price > 0 AND status = 'completed'), 0),
			coalesce(sum(price) FILTER (WHERE price > 0 AND status IN ('cancelled', 'no_show')), 0),
			count(*) FILTER (WHERE price > 0 AND status = 'completed')
		FROM consultations WHERE created_at BETWEEN @from AND @to`
	var completedCount int64
	if err := r.db.QueryRow(ctx, consultQ, pgx.NamedArgs{"from": from, "to": to}).
		Scan(&t.Consultations, &t.RevenueBooked, &t.RevenueEarned, &t.RevenueLost, &completedCount); err != nil {
		return t, fmt.Errorf("totals consultations: %w", err)
	}
	if completedCount > 0 {
		t.AvgTicket = t.RevenueEarned / float64(completedCount)
	}

	// Cases (дела) opened in the period — same "what happened this period"
	// scoping as consultations above. CasesInProgress/CaseOwed are
	// deliberately NOT in this query — see below.
	const casesQ = `
		SELECT
			count(*) FILTER (WHERE status = 'completed'),
			coalesce(sum(fee), 0),
			coalesce(sum(paid_amount), 0)
		FROM cases WHERE created_at BETWEEN @from AND @to`
	if err := r.db.QueryRow(ctx, casesQ, pgx.NamedArgs{"from": from, "to": to}).
		Scan(&t.CasesCompleted, &t.CaseFeeContracted, &t.CasePaid); err != nil {
		return t, fmt.Errorf("totals cases: %w", err)
	}

	// CasesInProgress/CaseOwed are a live snapshot across every case, not
	// scoped to [from, to] — "how many cases are active" and "how much are
	// we owed" describe the business's current state, not what happened in
	// a window; a case opened last month and still unpaid is real debt
	// today, and a short date range shouldn't make it disappear. CaseOwed
	// only counts cases still short of their fee — one paid in full or
	// overpaid contributes 0, never a negative "owed".
	const liveCasesQ = `
		SELECT
			count(*) FILTER (WHERE status = 'in_progress'),
			coalesce(sum(greatest(fee - paid_amount, 0)), 0)
		FROM cases`
	if err := r.db.QueryRow(ctx, liveCasesQ).Scan(&t.CasesInProgress, &t.CaseOwed); err != nil {
		return t, fmt.Errorf("totals cases live: %w", err)
	}
	return t, nil
}

// Trend buckets leads (by received_at) and consultations (by created_at)
// into the same day/month buckets and full-outer-joins them, so a bucket
// with only one of the two still shows up with 0 on the other side.
// RevenueEarned per bucket only counts completed consultations — same
// reasoning as Totals.
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
			SELECT date_trunc('%s', created_at) AS bucket,
				count(*) AS consultations,
				coalesce(sum(price) FILTER (WHERE price > 0 AND status = 'completed'), 0) AS revenue_earned
			FROM consultations WHERE created_at BETWEEN @from AND @to
			GROUP BY 1
		)
		SELECT coalesce(l.bucket, c.bucket) AS bucket, coalesce(l.leads, 0), coalesce(c.consultations, 0), coalesce(c.revenue_earned, 0)
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
		if err := rows.Scan(&bucketAt, &b.Leads, &b.Consultations, &b.RevenueEarned); err != nil {
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

// BySource attributes each client to the page of their FIRST-ever lead
// (first-touch — DISTINCT ON ... ORDER BY received_at), restricted to
// clients whose first lead fell in [from, to], then follows those same
// clients to consultation/case revenue with no date bound of its own — same
// cohort shape as Funnel, just keyed by source instead of summed overall.
func (r *LeadStatsRepository) BySource(ctx context.Context, from, to time.Time) ([]leadstats.SourceValue, error) {
	const q = `
		WITH cohort AS (
			SELECT DISTINCT ON (client_id) client_id, page, received_at
			FROM leads
			WHERE client_id IS NOT NULL
			ORDER BY client_id, received_at ASC
		),
		cohort_in_range AS (
			SELECT client_id, page FROM cohort WHERE received_at BETWEEN @from AND @to
		),
		consult_agg AS (
			SELECT client_id,
				coalesce(sum(price) FILTER (WHERE price > 0 AND status = 'completed'), 0) AS revenue
			FROM consultations GROUP BY client_id
		),
		case_agg AS (
			SELECT client_id, coalesce(sum(paid_amount), 0) AS paid
			FROM cases GROUP BY client_id
		)
		SELECT
			cir.page,
			count(*),
			count(ca.client_id),
			count(cs.client_id),
			coalesce(sum(ca.revenue), 0),
			coalesce(sum(cs.paid), 0)
		FROM cohort_in_range cir
		LEFT JOIN consult_agg ca ON ca.client_id = cir.client_id
		LEFT JOIN case_agg cs ON cs.client_id = cir.client_id
		GROUP BY cir.page
		ORDER BY (coalesce(sum(ca.revenue), 0) + coalesce(sum(cs.paid), 0)) DESC`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"from": from, "to": to})
	if err != nil {
		return nil, fmt.Errorf("by source: %w", err)
	}
	defer rows.Close()

	var out []leadstats.SourceValue
	for rows.Next() {
		var s leadstats.SourceValue
		if err := rows.Scan(&s.Key, &s.Leads, &s.ConsultedEver, &s.CasedEver, &s.RevenueEarned, &s.CasePaid); err != nil {
			return nil, fmt.Errorf("by source: scan: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("by source: %w", err)
	}
	return out, nil
}

// ByCreator ranks staff by earned revenue, not booking volume — a plain
// count rewards whoever books the most regardless of how many of those
// bookings actually happened.
func (r *LeadStatsRepository) ByCreator(ctx context.Context, from, to time.Time) ([]leadstats.CreatorRevenue, error) {
	const q = `
		SELECT created_by, count(*), coalesce(sum(price) FILTER (WHERE price > 0 AND status = 'completed'), 0)
		FROM consultations
		WHERE created_at BETWEEN @from AND @to
		GROUP BY created_by ORDER BY 3 DESC, count(*) DESC`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"from": from, "to": to})
	if err != nil {
		return nil, fmt.Errorf("by creator: %w", err)
	}
	defer rows.Close()

	var out []leadstats.CreatorRevenue
	for rows.Next() {
		var c leadstats.CreatorRevenue
		if err := rows.Scan(&c.Key, &c.Bookings, &c.RevenueEarned); err != nil {
			return nil, fmt.Errorf("by creator: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("by creator: %w", err)
	}
	return out, nil
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

// ByCaseCategory answers "which direction actually makes money" — grouped
// by created_at same as everything else in this file, empty category
// (nothing picked in the bot flow, or an un-categorized historical import)
// shows up as its own row rather than silently vanishing.
func (r *LeadStatsRepository) ByCaseCategory(ctx context.Context, from, to time.Time) ([]leadstats.CategoryRevenue, error) {
	const q = `
		SELECT category, count(*), coalesce(sum(fee), 0), coalesce(sum(paid_amount), 0)
		FROM cases
		WHERE created_at BETWEEN @from AND @to
		GROUP BY category ORDER BY 4 DESC, count(*) DESC`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"from": from, "to": to})
	if err != nil {
		return nil, fmt.Errorf("by case category: %w", err)
	}
	defer rows.Close()

	var out []leadstats.CategoryRevenue
	for rows.Next() {
		var c leadstats.CategoryRevenue
		if err := rows.Scan(&c.Key, &c.Cases, &c.Contracted, &c.Paid); err != nil {
			return nil, fmt.Errorf("by case category: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("by case category: %w", err)
	}
	return out, nil
}

// ByCaseAdvocate answers "who actually closes real money" — unlike
// ByCreator (consultation bookings), this is case revenue, grouped by
// advocate_name so it covers both historical free-text cases and ones
// created through the roster picker (see ABL 017/advocates roster).
func (r *LeadStatsRepository) ByCaseAdvocate(ctx context.Context, from, to time.Time) ([]leadstats.CategoryRevenue, error) {
	const q = `
		SELECT advocate_name, count(*), coalesce(sum(fee), 0), coalesce(sum(paid_amount), 0)
		FROM cases
		WHERE created_at BETWEEN @from AND @to
		GROUP BY advocate_name ORDER BY 4 DESC, count(*) DESC`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"from": from, "to": to})
	if err != nil {
		return nil, fmt.Errorf("by case advocate: %w", err)
	}
	defer rows.Close()

	var out []leadstats.CategoryRevenue
	for rows.Next() {
		var c leadstats.CategoryRevenue
		if err := rows.Scan(&c.Key, &c.Cases, &c.Contracted, &c.Paid); err != nil {
			return nil, fmt.Errorf("by case advocate: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("by case advocate: %w", err)
	}
	return out, nil
}

// Funnel is a cohort conversion, deliberately not reusing Totals' leads/
// consultations/cases counts — those are each filtered by their own date
// column independently, which is exactly the period-ratio problem the
// domain doc warns about. Here the cohort (clients whose first lead fell in
// range) is fixed once in the CTE, and every later stage checks that same
// client set with no date bound of its own.
func (r *LeadStatsRepository) Funnel(ctx context.Context, from, to time.Time) (leadstats.Funnel, error) {
	const q = `
		WITH cohort AS (
			SELECT client_id, min(received_at) AS first_lead_at
			FROM leads
			WHERE client_id IS NOT NULL
			GROUP BY client_id
			HAVING min(received_at) BETWEEN @from AND @to
		),
		first_consult AS (
			SELECT client_id, min(created_at) AS first_consult_at
			FROM consultations
			GROUP BY client_id
		),
		first_case AS (
			SELECT client_id, min(created_at) AS first_case_at
			FROM cases
			GROUP BY client_id
		)
		SELECT
			count(*),
			count(fc.client_id),
			count(fk.client_id),
			coalesce(avg(extract(epoch FROM (fc.first_consult_at - cohort.first_lead_at)) / 86400)
				FILTER (WHERE fc.client_id IS NOT NULL AND fc.first_consult_at >= cohort.first_lead_at), 0),
			coalesce(avg(extract(epoch FROM (fk.first_case_at - fc.first_consult_at)) / 86400)
				FILTER (WHERE fk.client_id IS NOT NULL AND fc.client_id IS NOT NULL AND fk.first_case_at >= fc.first_consult_at), 0)
		FROM cohort
		LEFT JOIN first_consult fc ON fc.client_id = cohort.client_id
		LEFT JOIN first_case fk ON fk.client_id = cohort.client_id`

	var f leadstats.Funnel
	if err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"from": from, "to": to}).Scan(
		&f.CohortLeads, &f.ConsultedEver, &f.CasedEver,
		&f.AvgDaysToConsult, &f.AvgDaysConsultToCase,
	); err != nil {
		return f, fmt.Errorf("funnel: %w", err)
	}
	return f, nil
}

// ByWeekday groups leads by ISO day-of-week (1 Monday .. 7 Sunday) — see
// the Stats.ByWeekday doc comment for why there's no by-hour counterpart.
func (r *LeadStatsRepository) ByWeekday(ctx context.Context, from, to time.Time) ([]leadstats.Count, error) {
	const q = `
		SELECT extract(isodow FROM received_at)::text, count(*)
		FROM leads WHERE received_at BETWEEN @from AND @to
		GROUP BY 1 ORDER BY 1`
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
