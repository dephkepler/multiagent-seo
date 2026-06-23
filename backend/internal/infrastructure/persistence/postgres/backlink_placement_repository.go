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
		INSERT INTO backlink_placements (run_id, sheet, donor_url, target_url, ok, status, post_url, edit_url, anchor)
		VALUES (@run_id, @sheet, @donor_url, @target_url, @ok, @status, @post_url, @edit_url, @anchor)`

	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"run_id":     p.RunID,
		"sheet":      p.Sheet,
		"donor_url":  p.DonorURL,
		"target_url": p.TargetURL,
		"ok":         p.OK,
		"status":     p.Status,
		"post_url":   p.PostURL,
		"edit_url":   p.EditURL,
		"anchor":     p.Anchor,
	}); err != nil {
		return fmt.Errorf("save backlink placement: %w", err)
	}
	return nil
}

func (r *BacklinkPlacementRepository) ListByRun(ctx context.Context, runID string) ([]linkbuilding.Placement, error) {
	const q = `
		SELECT id, run_id, sheet, donor_url, target_url, ok, status, post_url, edit_url, anchor, created_at
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
		if err := rows.Scan(&p.ID, &p.RunID, &p.Sheet, &p.DonorURL, &p.TargetURL, &p.OK, &p.Status, &p.PostURL, &p.EditURL, &p.Anchor, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan backlink placement: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
