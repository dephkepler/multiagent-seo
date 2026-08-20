package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/finance"
)

type AdvocateRateRepository struct {
	db *pgxpool.Pool
}

func NewAdvocateRateRepository(db *pgxpool.Pool) *AdvocateRateRepository {
	return &AdvocateRateRepository{db: db}
}

// Inactive advocates stay in the list: they still have collected money in past
// months, so their payout rate remains relevant to a historical report.
func (r *AdvocateRateRepository) ListAdvocateRates(ctx context.Context) ([]finance.AdvocateRate, error) {
	const q = `
		SELECT id, full_name, is_active, commission_percent
		FROM advocates
		ORDER BY is_active DESC, full_name`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query advocate rates: %w", err)
	}
	defer rows.Close()

	var out []finance.AdvocateRate
	for rows.Next() {
		var a finance.AdvocateRate
		if err := rows.Scan(&a.AdvocateID, &a.FullName, &a.IsActive, &a.CommissionPercent); err != nil {
			return nil, fmt.Errorf("scan advocate rate: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate advocate rates: %w", err)
	}
	return out, nil
}

func (r *AdvocateRateRepository) SetAdvocateRate(ctx context.Context, advocateID string, percent float64) error {
	const q = `UPDATE advocates SET commission_percent = $2 WHERE id = $1`

	tag, err := r.db.Exec(ctx, q, advocateID, percent)
	if err != nil {
		return fmt.Errorf("set advocate rate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return finance.ErrNotFound
	}
	return nil
}
