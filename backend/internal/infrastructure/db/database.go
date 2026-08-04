package db

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/pkg/config"
)

type Database struct {
	cfg     config.DatabaseConfig
	pool    *pgxpool.Pool
	once    sync.Once
	initErr error
}

func NewDatabase(cfg config.DatabaseConfig) *Database {
	return &Database{cfg: cfg}
}

func (d *Database) Initialize(ctx context.Context) error {
	d.once.Do(func() {
		pool, err := newPool(ctx, d.cfg)
		if err != nil {
			d.initErr = err
			return
		}
		if err := RunMigrations(ctx, d.cfg); err != nil {
			pool.Close()
			d.initErr = fmt.Errorf("run migrations: %w", err)
			return
		}
		d.pool = pool
	})
	return d.initErr
}

func (d *Database) Pool() *pgxpool.Pool {
	return d.pool
}

func (d *Database) Close() {
	if d.pool != nil {
		d.pool.Close()
		d.pool = nil
	}
}
