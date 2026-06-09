//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"multiagent-seo/internal/domain/user"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

func TestUserRepository_FindByEmail(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewUserRepository(pool)

	const (
		email = "user@example.com"
		hash  = "$2a$10$abcdefghijklmnopqrstuv"
	)
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash) VALUES (@email, @hash)`,
		pgx.NamedArgs{"email": email, "hash": hash}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	got, err := repo.FindByEmail(ctx, email)
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if got.Email != email {
		t.Errorf("Email = %q, want %q", got.Email, email)
	}
	if got.PasswordHash != hash {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, hash)
	}
	if got.ID.String() == "" {
		t.Errorf("ID not populated")
	}
}

func TestUserRepository_FindByEmailNotFound(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewUserRepository(testsupport.NewTestDB(t, baseConnStr))

	_, err := repo.FindByEmail(ctx, "missing@example.com")
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("FindByEmail missing err = %v, want ErrNotFound", err)
	}
}
