//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"multiagent-seo/internal/testsupport"
)

// TestSmoke_MigrationsApplied confirms the container boots, migrations run, and
// each test gets an isolated database with the expected schema.
func TestSmoke_MigrationsApplied(t *testing.T) {
	pool := testsupport.NewTestDB(t, baseConnStr)
	ctx := context.Background()

	for _, table := range []string{"articles", "users", "wordpress_sites"} {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table).Scan(&exists)
		if err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q missing after migrations", table)
		}
	}

	// pgcrypto must be installed (wordpress app-password encryption depends on it).
	var hasPgcrypto bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pgcrypto')`).Scan(&hasPgcrypto); err != nil {
		t.Fatalf("pgcrypto check: %v", err)
	}
	if !hasPgcrypto {
		t.Error("pgcrypto extension not installed by migrations")
	}
}
