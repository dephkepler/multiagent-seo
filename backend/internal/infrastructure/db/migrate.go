package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/logger"
)

func RunMigrations(ctx context.Context, cfg config.DatabaseConfig) error {
	log := logger.New(ctx, "infrastructure.db")

	dir := cfg.MigrationsDir
	if dir == "" {
		dir = "migrations"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("migrations path: %w", err)
	}
	sourceURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()

	m, err := migrate.New(sourceURL, FormatConnectionURL(cfg, true))
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info().Str("dir", abs).Msg("migrations up to date")
			return nil
		}
		return fmt.Errorf("migrate up: %w", err)
	}

	log.Info().Str("dir", abs).Msg("migrations applied")
	return nil
}
