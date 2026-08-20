package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/clientsegments"
)

type ClientSegmentsRepository struct {
	db *pgxpool.Pool
}

func NewClientSegmentsRepository(db *pgxpool.Pool) *ClientSegmentsRepository {
	return &ClientSegmentsRepository{db: db}
}

// LEFT JOINs matter: a client with a lead but no consultation/case must still show up, not be dropped.
func (r *ClientSegmentsRepository) ListActivity(ctx context.Context) ([]clientsegments.Activity, error) {
	const q = `
		SELECT
			cl.id, cl.name, coalesce(cl.phone, ''),
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
			cl.segment_override,
			coalesce(mt.tags, '{}')
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
		LEFT JOIN (
			SELECT client_id, array_agg(tag ORDER BY tag) AS tags
			FROM client_tags
			GROUP BY client_id
		) mt ON mt.client_id = cl.id
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
			&a.ManualTags,
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

// ON CONFLICT DO NOTHING (re-add = no-op); ConstraintName disambiguates the two FKs sharing SQLSTATE 23503.
func (r *ClientSegmentsRepository) AddTag(ctx context.Context, clientID, tag, createdBy string) error {
	const q = `INSERT INTO client_tags (client_id, tag, created_by) VALUES (@id, @tag, @by)
		ON CONFLICT (client_id, tag) DO NOTHING`
	_, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": clientID, "tag": tag, "by": createdBy})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			if pgErr.ConstraintName == "client_tags_tag_fkey" {
				return fmt.Errorf("add tag %q for %q: %w", tag, clientID, clientsegments.ErrUnknownTag)
			}
			return fmt.Errorf("add tag %q for %q: %w", tag, clientID, clientsegments.ErrNotFound)
		}
		return fmt.Errorf("add tag %q for %q: %w", tag, clientID, err)
	}
	return nil
}

func (r *ClientSegmentsRepository) RemoveTag(ctx context.Context, clientID, tag string) error {
	const q = `DELETE FROM client_tags WHERE client_id = @id AND tag = @tag`
	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": clientID, "tag": tag}); err != nil {
		return fmt.Errorf("remove tag %q for %q: %w", tag, clientID, err)
	}
	return nil
}

// ClientTags is a direct indexed lookup (client_tags' primary key leads
// with client_id) — unlike List, it never touches consultations/cases or
// runs Derive for every other client just to answer "what tags does this
// one client have."
func (r *ClientSegmentsRepository) ClientTags(ctx context.Context, clientID string) ([]string, error) {
	const q = `SELECT tag FROM client_tags WHERE client_id = @id ORDER BY tag`
	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"id": clientID})
	if err != nil {
		return nil, fmt.Errorf("client tags %q: %w", clientID, err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("client tags %q: scan: %w", clientID, err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("client tags %q: %w", clientID, err)
	}
	return tags, nil
}

func (r *ClientSegmentsRepository) ListTagDefs(ctx context.Context) ([]clientsegments.TagDef, error) {
	const q = `SELECT label, category, created_at FROM client_tag_defs ORDER BY category, label`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list tag defs: %w", err)
	}
	defer rows.Close()

	var out []clientsegments.TagDef
	for rows.Next() {
		var d clientsegments.TagDef
		if err := rows.Scan(&d.Label, &d.Category, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("list tag defs: scan: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tag defs: %w", err)
	}
	return out, nil
}

func (r *ClientSegmentsRepository) CreateTagDef(ctx context.Context, label, category, createdBy string) error {
	const q = `INSERT INTO client_tag_defs (label, category, created_by) VALUES (@label, @category, @by)`
	_, err := r.db.Exec(ctx, q, pgx.NamedArgs{"label": label, "category": category, "by": createdBy})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("create tag def %q: %w", label, clientsegments.ErrTagDefExists)
		}
		return fmt.Errorf("create tag def %q: %w", label, err)
	}
	return nil
}

// UpdateTagDef always runs the same statement — coalesce(@new, current)
// leaves a field untouched when its pointer is nil — rather than building
// the SET clause dynamically for what's still just two possible columns.
func (r *ClientSegmentsRepository) UpdateTagDef(ctx context.Context, label string, newLabel, newCategory *string) error {
	const q = `UPDATE client_tag_defs SET
			label = coalesce(@new_label, label),
			category = coalesce(@new_category, category)
		WHERE label = @label`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"label": label, "new_label": newLabel, "new_category": newCategory})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("update tag def %q: %w", label, clientsegments.ErrTagDefExists)
		}
		return fmt.Errorf("update tag def %q: %w", label, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update tag def %q: %w", label, clientsegments.ErrTagDefNotFound)
	}
	return nil
}

// idempotent — ON DELETE CASCADE erases the trace, so "already gone" and "just removed" look the same.
func (r *ClientSegmentsRepository) DeleteTagDef(ctx context.Context, label string) error {
	const q = `DELETE FROM client_tag_defs WHERE label = @label`
	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{"label": label}); err != nil {
		return fmt.Errorf("delete tag def %q: %w", label, err)
	}
	return nil
}
