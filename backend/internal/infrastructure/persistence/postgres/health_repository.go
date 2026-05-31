package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type HealthRepository struct {
	db *pgxpool.Pool
}

func NewHealthRepository(db *pgxpool.Pool) *HealthRepository {
	return &HealthRepository{db: db}
}

func (r *HealthRepository) Ping(ctx context.Context) error {
	return r.db.Ping(ctx)
}
