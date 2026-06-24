package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/linkbuilding"
)

type DonorSiteProfileRepository struct {
	db *pgxpool.Pool
}

func NewDonorSiteProfileRepository(db *pgxpool.Pool) *DonorSiteProfileRepository {
	return &DonorSiteProfileRepository{db: db}
}

func (r *DonorSiteProfileRepository) Save(ctx context.Context, p linkbuilding.DonorProfile) error {
	const q = `
		INSERT INTO donor_site_profiles
			(donor_url, role, can_edit_pages, can_edit_others, can_publish, can_create, last_outcome)
		VALUES
			(@donor_url, @role, @can_edit_pages, @can_edit_others, @can_publish, @can_create, @last_outcome)
		ON CONFLICT (donor_url) DO UPDATE SET
			role            = EXCLUDED.role,
			can_edit_pages  = EXCLUDED.can_edit_pages,
			can_edit_others = EXCLUDED.can_edit_others,
			can_publish     = EXCLUDED.can_publish,
			can_create      = EXCLUDED.can_create,
			last_outcome    = EXCLUDED.last_outcome,
			updated_at      = now()`

	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"donor_url":       p.DonorURL,
		"role":            p.Role,
		"can_edit_pages":  p.CanEditPages,
		"can_edit_others": p.CanEditOthers,
		"can_publish":     p.CanPublish,
		"can_create":      p.CanCreate,
		"last_outcome":    p.LastOutcome,
	}); err != nil {
		return fmt.Errorf("save donor profile: %w", err)
	}
	return nil
}

func (r *DonorSiteProfileRepository) RecentlyFailed(ctx context.Context, lockedCutoff, failCutoff time.Time) (map[string]bool, error) {
	const q = `
		SELECT donor_url FROM donor_site_profiles
		WHERE (last_outcome = 'locked' AND updated_at > @locked_cutoff)
		   OR (last_outcome IN ('cloudflare', 'login_failed', 'no_marker', 'post_failed', 'link_missing') AND updated_at > @fail_cutoff)`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"locked_cutoff": lockedCutoff, "fail_cutoff": failCutoff})
	if err != nil {
		return nil, fmt.Errorf("list recently failed donors: %w", err)
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var donorURL string
		if err := rows.Scan(&donorURL); err != nil {
			return nil, fmt.Errorf("scan recently failed donor: %w", err)
		}
		out[linkbuilding.NormalizeDonorURL(donorURL)] = true
	}
	return out, rows.Err()
}
