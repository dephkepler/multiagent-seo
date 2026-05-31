//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"

	"multiagent-seo/internal/testsupport"
)

// baseConnStr points at the shared container's default database; NewTestDB
// clones an isolated per-test database from the migrated template.
var baseConnStr string

func TestMain(m *testing.M) {
	ctx := context.Background()
	conn, terminate, err := testsupport.StartPostgres(ctx)
	if err != nil {
		panic(err)
	}
	baseConnStr = conn
	code := m.Run()
	terminate()
	os.Exit(code)
}
