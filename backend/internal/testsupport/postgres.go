//go:build integration

package testsupport

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func StartPostgres(ctx context.Context) (baseConnStr string, terminate func(), err error) {
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithDatabase("app"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start container: %w", err)
	}
	stop := func() { _ = container.Terminate(context.Background()) }

	baseConnStr, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		stop()
		return "", nil, fmt.Errorf("conn string: %w", err)
	}

	if err := buildTemplate(ctx, baseConnStr); err != nil {
		stop()
		return "", nil, err
	}
	return baseConnStr, stop, nil
}

func buildTemplate(ctx context.Context, base string) error {
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		return fmt.Errorf("admin pool: %w", err)
	}
	_, err = admin.Exec(ctx, `CREATE DATABASE test_template`)
	admin.Close()
	if err != nil {
		return fmt.Errorf("create template db: %w", err)
	}
	return applyMigrations(ctx, connStrForDB(base, "test_template"))
}

func applyMigrations(ctx context.Context, connStr string) error {
	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("migrate pool: %w", err)
	}
	defer pool.Close()

	files, err := upMigrationFiles()
	if err != nil {
		return err
	}
	for _, f := range files {
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", filepath.Base(f), err)
		}
	}
	return nil
}

func upMigrationFiles() ([]string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("runtime.Caller failed")
	}
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	matches, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		return nil, fmt.Errorf("glob migrations: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no migrations found in %s", migrationsDir)
	}
	sort.Strings(matches)
	return matches, nil
}

func NewTestDB(t *testing.T, baseConnStr string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	dbName := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	dbIdent := pgx.Identifier{dbName}.Sanitize()

	admin, err := pgxpool.New(ctx, baseConnStr)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	_, err = admin.Exec(ctx, `CREATE DATABASE `+dbIdent+` TEMPLATE test_template`)
	admin.Close()
	if err != nil {
		t.Fatalf("clone template: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStrForDB(baseConnStr, dbName))
	if err != nil {
		t.Fatalf("test pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if admin, err := pgxpool.New(context.Background(), baseConnStr); err == nil {
			_, _ = admin.Exec(context.Background(), `DROP DATABASE `+dbIdent)
			admin.Close()
		}
	})
	return pool
}

func connStrForDB(base, dbName string) string {
	u, err := url.Parse(base)
	if err != nil {
		panic(err)
	}
	u.Path = "/" + dbName
	return u.String()
}
