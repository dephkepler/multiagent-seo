package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/logger"
)

func newPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	log := logger.New(ctx, "infrastructure.db")

	pool, err := pgxpool.New(ctx, FormatConnectionURL(cfg, false))
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Info().Str("host", cfg.Host).Str("dbname", cfg.Dbname).Msg("connected to database")
	return pool, nil
}
