//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

// Reproduces a prod symptom: /advocate in the bot replies "saved" (no error
// from CreateAdvocate), yet a fresh psql session against the same database
// sees zero rows in `advocates` (pg_stat_user_tables.n_tup_ins stuck at 0).
// This writes through one pool and reads back through a completely separate
// pool/connection — same thing a fresh psql session does — to see whether
// the write is actually durable, or only visible to the connection that
// made it (which would point at an uncommitted transaction somewhere).
//
// Turned out not to be a code bug at all (see ABL 029 in Obsidian) — a
// stray local `cmd/server` was holding the bot's long-poll lock, so every
// write was landing in the local dev database, not prod. Kept as a
// regression test anyway: it's a real guarantee worth pinning down.
func TestConsultationRepository_CreateAdvocate_VisibleFromAnotherConnection(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewConsultationRepository(pool)

	saved, err := repo.CreateAdvocate(ctx, "Ярослав Борзов")
	if err != nil {
		t.Fatalf("CreateAdvocate: %v", err)
	}
	if saved.ID == "" {
		t.Fatalf("CreateAdvocate returned empty ID")
	}
	if !saved.IsActive {
		t.Errorf("IsActive = false, want true for a freshly created advocate")
	}

	// Independent pool — a brand new connection, exactly what a fresh
	// `psql -c "SELECT ..."` session is.
	other, err := pgxpool.NewWithConfig(ctx, pool.Config())
	if err != nil {
		t.Fatalf("second pool: %v", err)
	}
	defer other.Close()

	var count int
	if err := other.QueryRow(ctx, `SELECT count(*) FROM advocates`).Scan(&count); err != nil {
		t.Fatalf("count via second connection: %v", err)
	}
	if count != 1 {
		t.Errorf("advocates visible from a second connection = %d, want 1 (write from the first connection isn't durable)", count)
	}

	var fullName string
	if err := other.QueryRow(ctx, `SELECT full_name FROM advocates WHERE id = @id`,
		pgx.NamedArgs{"id": saved.ID}).Scan(&fullName); err != nil {
		t.Fatalf("select by id via second connection: %v", err)
	}
	if fullName != "Ярослав Борзов" {
		t.Errorf("full_name = %q, want %q", fullName, "Ярослав Борзов")
	}
}

// Advocates are a roster now, not a single overwritten slot — calling
// CreateAdvocate twice must produce two distinct rows, not update the first.
func TestConsultationRepository_CreateAdvocate_CalledTwiceMakesTwoRows(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewConsultationRepository(pool)

	first, err := repo.CreateAdvocate(ctx, "Ярослав Борзов")
	if err != nil {
		t.Fatalf("first CreateAdvocate: %v", err)
	}
	second, err := repo.CreateAdvocate(ctx, "Ганна Коваль")
	if err != nil {
		t.Fatalf("second CreateAdvocate: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("second call reused the first row: both have id %s", first.ID)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM advocates`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("advocates row count = %d, want 2", count)
	}
}

// DeactivateAdvocate is a soft-delete: the advocate drops out of
// ListAdvocates(activeOnly=true) but the row itself, and anything already
// linked to it, is untouched.
func TestConsultationRepository_DeactivateAdvocate(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewConsultationRepository(pool)

	a, err := repo.CreateAdvocate(ctx, "Ярослав Борзов")
	if err != nil {
		t.Fatalf("CreateAdvocate: %v", err)
	}

	if err := repo.DeactivateAdvocate(ctx, a.ID); err != nil {
		t.Fatalf("DeactivateAdvocate: %v", err)
	}

	active, err := repo.ListAdvocates(ctx, true)
	if err != nil {
		t.Fatalf("ListAdvocates(true): %v", err)
	}
	if len(active) != 0 {
		t.Errorf("ListAdvocates(true) after deactivate = %d advocates, want 0", len(active))
	}

	all, err := repo.ListAdvocates(ctx, false)
	if err != nil {
		t.Fatalf("ListAdvocates(false): %v", err)
	}
	if len(all) != 1 {
		t.Errorf("ListAdvocates(false) after deactivate = %d advocates, want 1 (row should still exist)", len(all))
	}
}
