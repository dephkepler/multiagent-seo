//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/user"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

func seedClientWithChat(t *testing.T, pool *pgxpool.Pool, phone string, chatID any) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO clients (phone, name, telegram_chat_id) VALUES (@phone, 'Клієнт', @chat) RETURNING id::text`,
		pgx.NamedArgs{"phone": phone, "chat": chatID}).Scan(&id)
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	return id
}

func seedAdvocateWithChat(t *testing.T, pool *pgxpool.Pool, name string, chatID any, active bool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO advocates (full_name, telegram_chat_id, is_active) VALUES (@name, @chat, @active) RETURNING id::text`,
		pgx.NamedArgs{"name": name, "chat": chatID, "active": active}).Scan(&id)
	if err != nil {
		t.Fatalf("seed advocate: %v", err)
	}
	return id
}

func TestTelegramRepository_FindsAClient(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewTelegramRepository(pool)

	const chatID int64 = 555001
	clientID := seedClientWithChat(t, pool, "+380990000001", chatID)

	got, err := repo.FindByTelegramID(ctx, chatID)
	if err != nil {
		t.Fatalf("FindByTelegramID: %v", err)
	}
	if got.Role != user.RoleClient {
		t.Errorf("Role = %q, want client", got.Role)
	}
	if got.ClientID != clientID {
		t.Errorf("ClientID = %q, want %q", got.ClientID, clientID)
	}
	if got.AdvocateID != "" {
		t.Errorf("AdvocateID = %q, want empty", got.AdvocateID)
	}
}

func TestTelegramRepository_FindsAnAdvocate(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewTelegramRepository(pool)

	const chatID int64 = 555002
	advocateID := seedAdvocateWithChat(t, pool, "Адвокат", chatID, true)

	got, err := repo.FindByTelegramID(ctx, chatID)
	if err != nil {
		t.Fatalf("FindByTelegramID: %v", err)
	}
	if got.Role != user.RoleAdvocate {
		t.Errorf("Role = %q, want advocate", got.Role)
	}
	if got.AdvocateID != advocateID {
		t.Errorf("AdvocateID = %q, want %q", got.AdvocateID, advocateID)
	}
}

// An advocate who was once a client of the firm has a row in both tables. The
// roster is the more privileged reading, so it wins — and it must win by an
// explicit priority, not by which table the query happened to list first.
func TestTelegramRepository_RosterWinsOverTheClientList(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewTelegramRepository(pool)

	const chatID int64 = 555003
	seedClientWithChat(t, pool, "+380990000003", chatID)
	advocateID := seedAdvocateWithChat(t, pool, "Адвокат і клієнт", chatID, true)

	got, err := repo.FindByTelegramID(ctx, chatID)
	if err != nil {
		t.Fatalf("FindByTelegramID: %v", err)
	}
	if got.Role != user.RoleAdvocate || got.AdvocateID != advocateID {
		t.Errorf("got %+v, want the advocate row %q", got, advocateID)
	}
}

// Leaving the firm keeps the row — cases and consultations still point at it —
// but stops it from authenticating anyone.
func TestTelegramRepository_InactiveAdvocateDoesNotAuthenticate(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewTelegramRepository(pool)

	const chatID int64 = 555004
	seedAdvocateWithChat(t, pool, "Колишній адвокат", chatID, false)

	_, err := repo.FindByTelegramID(ctx, chatID)
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// A deactivated advocate who is also a client keeps the client reading, which
// is the whole reason the roster query filters instead of the outer one.
func TestTelegramRepository_InactiveAdvocateFallsBackToTheirClientRow(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewTelegramRepository(pool)

	const chatID int64 = 555005
	clientID := seedClientWithChat(t, pool, "+380990000005", chatID)
	seedAdvocateWithChat(t, pool, "Колишній адвокат і клієнт", chatID, false)

	got, err := repo.FindByTelegramID(ctx, chatID)
	if err != nil {
		t.Fatalf("FindByTelegramID: %v", err)
	}
	if got.Role != user.RoleClient || got.ClientID != clientID {
		t.Errorf("got %+v, want the client row %q", got, clientID)
	}
}

func TestTelegramRepository_UnknownIDIsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewTelegramRepository(pool)

	_, err := repo.FindByTelegramID(ctx, 555999)
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// The index from 000057 is what makes "who is calling" have one answer. Without
// it two client rows could carry the same chat id and authentication would pick
// one arbitrarily.
func TestTelegramRepository_ChatIDCannotBeSharedByTwoClients(t *testing.T) {
	pool := testsupport.NewTestDB(t, baseConnStr)

	const chatID int64 = 555006
	seedClientWithChat(t, pool, "+380990000006", chatID)

	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO clients (phone, name, telegram_chat_id) VALUES (@phone, 'Дубль', @chat) RETURNING id::text`,
		pgx.NamedArgs{"phone": "+380990000007", "chat": chatID}).Scan(&id)
	if err == nil {
		t.Fatal("a second client took the same telegram_chat_id")
	}
}

// Most client rows have no chat id at all, and repeated NULLs must stay legal
// or every client without Telegram would collide with the next.
func TestTelegramRepository_ManyClientsMayHaveNoChatID(t *testing.T) {
	pool := testsupport.NewTestDB(t, baseConnStr)

	seedClientWithChat(t, pool, "+380990000008", nil)
	seedClientWithChat(t, pool, "+380990000009", nil)
}
