package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/linkbuilding"
)

type BacklinkPlacementRepository struct {
	db *pgxpool.Pool
}

func NewBacklinkPlacementRepository(db *pgxpool.Pool) *BacklinkPlacementRepository {
	return &BacklinkPlacementRepository{db: db}
}

func (r *BacklinkPlacementRepository) Save(ctx context.Context, p linkbuilding.Placement) error {
	const q = `
		INSERT INTO backlink_placements (run_id, sheet, donor_url, target_url, ok, outcome, status, post_url, edit_url, anchor, link_check)
		VALUES (@run_id, @sheet, @donor_url, @target_url, @ok, @outcome, @status, @post_url, @edit_url, @anchor, @link_check)`

	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"run_id":     p.RunID,
		"sheet":      p.Sheet,
		"donor_url":  p.DonorURL,
		"target_url": p.TargetURL,
		"ok":         p.OK,
		"outcome":    p.Outcome,
		"status":     p.Status,
		"post_url":   p.PostURL,
		"edit_url":   p.EditURL,
		"anchor":     p.Anchor,
		"link_check": p.LinkCheck,
	}); err != nil {
		return fmt.Errorf("save backlink placement: %w", err)
	}
	return nil
}

func (r *BacklinkPlacementRepository) ListByRun(ctx context.Context, runID string) ([]linkbuilding.Placement, error) {
	const q = `
		SELECT id, run_id, sheet, donor_url, target_url, ok, outcome, status, post_url, edit_url, anchor, link_check, created_at
		FROM backlink_placements
		WHERE run_id = @run_id
		ORDER BY created_at`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"run_id": runID})
	if err != nil {
		return nil, fmt.Errorf("list backlink placements: %w", err)
	}
	defer rows.Close()

	out := make([]linkbuilding.Placement, 0)
	for rows.Next() {
		var p linkbuilding.Placement
		if err := rows.Scan(&p.ID, &p.RunID, &p.Sheet, &p.DonorURL, &p.TargetURL, &p.OK, &p.Outcome, &p.Status, &p.PostURL, &p.EditURL, &p.Anchor, &p.LinkCheck, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan backlink placement: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *BacklinkPlacementRepository) ListPlaced(ctx context.Context, limit, offset int) ([]linkbuilding.Placement, int, error) {
	const q = `
		SELECT id, run_id, sheet, donor_url, target_url, ok, outcome, status, post_url, edit_url, anchor, link_check, created_at,
		       count(*) OVER () AS total
		FROM backlink_placements
		WHERE ok = true
		ORDER BY created_at DESC
		LIMIT @limit OFFSET @offset`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"limit": limit, "offset": offset})
	if err != nil {
		return nil, 0, fmt.Errorf("list placed: %w", err)
	}
	defer rows.Close()

	var total int
	out := make([]linkbuilding.Placement, 0, limit)
	for rows.Next() {
		var p linkbuilding.Placement
		if err := rows.Scan(&p.ID, &p.RunID, &p.Sheet, &p.DonorURL, &p.TargetURL, &p.OK, &p.Outcome, &p.Status, &p.PostURL, &p.EditURL, &p.Anchor, &p.LinkCheck, &p.CreatedAt, &total); err != nil {
			return nil, 0, fmt.Errorf("scan placed: %w", err)
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func (r *BacklinkPlacementRepository) PlacedDonors(ctx context.Context, targetURL string) (map[string]bool, error) {
	const q = `
		SELECT DISTINCT donor_url
		FROM backlink_placements
		WHERE target_url = @target_url AND ok = true`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"target_url": targetURL})
	if err != nil {
		return nil, fmt.Errorf("list placed donors: %w", err)
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var donorURL string
		if err := rows.Scan(&donorURL); err != nil {
			return nil, fmt.Errorf("scan placed donor: %w", err)
		}
		out[linkbuilding.NormalizeDonorURL(donorURL)] = true
	}
	return out, rows.Err()
}
