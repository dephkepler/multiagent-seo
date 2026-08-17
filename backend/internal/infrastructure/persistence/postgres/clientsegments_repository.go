package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/clientsegments"
)

type ClientSegmentsRepository struct {
	db *pgxpool.Pool
}

func NewClientSegmentsRepository(db *pgxpool.Pool) *ClientSegmentsRepository {
	return &ClientSegmentsRepository{db: db}
}

// ListActivity reads one row per client with everything Derive needs to
// classify them. Consultations and cases are aggregated in their own
// subquery each (a client can have many of both) and left-joined back onto
// clients, so someone with a lead but no consultation yet still shows up
// with zero counts instead of being dropped.
func (r *ClientSegmentsRepository) ListActivity(ctx context.Context) ([]clientsegments.Activity, error) {
	const q = `
		SELECT
			cl.id, cl.name, cl.phone,
			coalesce(co.completed_n, 0),
			coalesce(co.scheduled_n, 0),
			coalesce(co.lost_n, 0),
			coalesce(co.total_n, 0),
			coalesce(co.revenue, 0),
			coalesce(ca.case_n, 0),
			coalesce(ca.fee_sum, 0),
			coalesce(ca.paid_sum, 0),
			greatest(
				cl.first_seen_at,
				cl.last_seen_at,
				coalesce(co.last_at, cl.first_seen_at),
				coalesce(ca.last_at, cl.first_seen_at)
			),
			cl.segment_override
		FROM clients cl
		LEFT JOIN (
			SELECT
				client_id,
				count(*) FILTER (WHERE status = 'completed') AS completed_n,
				count(*) FILTER (WHERE status = 'scheduled') AS scheduled_n,
				count(*) FILTER (WHERE status IN ('cancelled', 'no_show')) AS lost_n,
				count(*) AS total_n,
				coalesce(sum(price) FILTER (WHERE price > 0 AND status = 'completed'), 0) AS revenue,
				max(created_at) AS last_at
			FROM consultations
			GROUP BY client_id
		) co ON co.client_id = cl.id
		LEFT JOIN (
			SELECT
				client_id,
				count(*) AS case_n,
				sum(fee) AS fee_sum,
				sum(paid_amount) AS paid_sum,
				max(created_at) AS last_at
			FROM cases
			GROUP BY client_id
		) ca ON ca.client_id = cl.id
		ORDER BY cl.first_seen_at DESC`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	defer rows.Close()

	var out []clientsegments.Activity
	for rows.Next() {
		var a clientsegments.Activity
		if err := rows.Scan(
			&a.ClientID, &a.Name, &a.Phone,
			&a.CompletedCount, &a.ScheduledCount, &a.LostCount, &a.ConsultCount,
			&a.ConsultRevenue,
			&a.CaseCount, &a.CaseFee, &a.CasePaid,
			&a.LastActivity,
			&a.SegmentOverride,
		); err != nil {
			return nil, fmt.Errorf("list activity: scan: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	return out, nil
}

// SetSegmentOverride is the write half of the override — see the
// clientsegments package doc for why it exists.
func (r *ClientSegmentsRepository) SetSegmentOverride(ctx context.Context, clientID string, segment *string) error {
	const q = `UPDATE clients SET segment_override = @segment WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"segment": segment, "id": clientID})
	if err != nil {
		return fmt.Errorf("set segment override %q: %w", clientID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set segment override %q: %w", clientID, clientsegments.ErrNotFound)
	}
	return nil
}
