//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/consultations"
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
// any non-empty key works here: the tests only round-trip through pgcrypto,
// they never assert the ciphertext.
const testEncryptionKey = "test-key"

func TestConsultationRepository_CreateAdvocate_VisibleFromAnotherConnection(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewConsultationRepository(pool, testEncryptionKey)

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
	repo := postgres.NewConsultationRepository(pool, testEncryptionKey)

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
	repo := postgres.NewConsultationRepository(pool, testEncryptionKey)

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

// The web CRM's client card lets staff hand-edit a client's phone; editing
// it to a number another client already has must fail with a clear
// consultations.ErrPhoneInUse, not a raw unique-violation from
// uq_clients_phone (see clientdetail handler's error map, which turns this
// into a 409 instead of a bare 500).
func TestConsultationRepository_UpdateClient_DuplicatePhoneFails(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewConsultationRepository(pool, testEncryptionKey)

	existing, err := repo.CreateClient(ctx, "Ганна Коваль", "+380501112222", "", "")
	if err != nil {
		t.Fatalf("CreateClient(existing): %v", err)
	}
	other, err := repo.CreateClient(ctx, "Ярослав Борзов", "+380503334444", "", "")
	if err != nil {
		t.Fatalf("CreateClient(other): %v", err)
	}

	err = repo.UpdateClient(ctx, other.ID, consultations.ClientEdit{
		FirstName:  "Ярослав",
		Phone:      existing.Phone,
		ClientType: consultations.ClientTypeIndividual,
	})
	if !errors.Is(err, consultations.ErrPhoneInUse) {
		t.Fatalf("UpdateClient with a phone already in use: err = %v, want errors.Is(err, consultations.ErrPhoneInUse)", err)
	}
}

// One Telegram account is one human, and 000057 lets its chat id point at only
// one client row. The same human gets a second row whenever they file another
// intake under a different phone, so the binding has to move instead of
// failing — otherwise their newest request is the one that cannot be recognised.
func TestConsultationRepository_SetClientTelegram_MovesTheBinding(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewConsultationRepository(pool, testEncryptionKey)

	const chatID int64 = 770001
	first, err := repo.CreateClient(ctx, "Перша заявка", "+380507770001", "", "")
	if err != nil {
		t.Fatalf("CreateClient(first): %v", err)
	}
	second, err := repo.CreateClient(ctx, "Друга заявка", "+380507770002", "", "")
	if err != nil {
		t.Fatalf("CreateClient(second): %v", err)
	}

	if err := repo.SetClientTelegram(ctx, first.ID, chatID, "@petro"); err != nil {
		t.Fatalf("SetClientTelegram(first): %v", err)
	}
	if err := repo.SetClientTelegram(ctx, second.ID, chatID, "@petro"); err != nil {
		t.Fatalf("SetClientTelegram(second): %v", err)
	}

	subjects := postgres.NewTelegramRepository(pool)
	got, err := subjects.FindByTelegramID(ctx, chatID)
	if err != nil {
		t.Fatalf("FindByTelegramID: %v", err)
	}
	if got.ClientID != second.ID {
		t.Errorf("chat resolves to %q, want the newest row %q", got.ClientID, second.ID)
	}

	var released *int64
	if err := pool.QueryRow(ctx,
		`SELECT telegram_chat_id FROM clients WHERE id = @id`,
		pgx.NamedArgs{"id": first.ID}).Scan(&released); err != nil {
		t.Fatalf("read the released row: %v", err)
	}
	if released != nil {
		t.Errorf("first client still holds chat id %d", *released)
	}
}

// The common case: the same client fills the form again with the same phone, so
// CreateClient returns the row they already have and the binding is re-applied
// to itself. The release step must not strip the id it is about to set.
func TestConsultationRepository_SetClientTelegram_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewConsultationRepository(pool, testEncryptionKey)

	const chatID int64 = 770002
	client, err := repo.CreateClient(ctx, "Повторна заявка", "+380507770003", "", "")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	for i := range 2 {
		if err := repo.SetClientTelegram(ctx, client.ID, chatID, "@petro"); err != nil {
			t.Fatalf("SetClientTelegram (call %d): %v", i+1, err)
		}
	}

	subjects := postgres.NewTelegramRepository(pool)
	got, err := subjects.FindByTelegramID(ctx, chatID)
	if err != nil {
		t.Fatalf("FindByTelegramID: %v", err)
	}
	if got.ClientID != client.ID {
		t.Errorf("chat resolves to %q, want %q", got.ClientID, client.ID)
	}
}

// A request holds a slot as firmly as a confirmed booking: otherwise two clients
// pick the same hour while the firm is still looking at the first one. Anything
// already resolved — completed, cancelled, no-show — holds nothing.
func TestConsultationRepository_HeldSlots_CountsRequestsAndBookings(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewConsultationRepository(pool, testEncryptionKey)

	client, err := repo.CreateClient(ctx, "Клієнт зі слотами", "+380507770010", "", "")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	day := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)
	seed := func(hour int, status string) time.Time {
		t.Helper()
		at := day.Add(time.Duration(hour) * time.Hour)
		if _, err := pool.Exec(ctx,
			`INSERT INTO consultations (client_id, scheduled_at, price, status)
				VALUES (@client, @at, 0, @status)`,
			pgx.NamedArgs{"client": client.ID, "at": at, "status": status}); err != nil {
			t.Fatalf("seed %s consultation: %v", status, err)
		}
		return at
	}

	requested := seed(10, consultations.StatusRequested)
	scheduled := seed(11, consultations.StatusScheduled)
	seed(12, consultations.StatusCompleted)
	seed(13, consultations.StatusCancelled)
	seed(14, consultations.StatusNoShow)
	// Outside the window asked for, so it must not come back either.
	seed(38, consultations.StatusScheduled)

	held, err := repo.HeldSlots(ctx, day, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("HeldSlots: %v", err)
	}

	want := map[int64]string{
		requested.Unix(): "requested",
		scheduled.Unix(): "scheduled",
	}
	if len(held) != len(want) {
		t.Fatalf("got %d held slots, want %d (%v)", len(held), len(want), held)
	}
	for _, slot := range held {
		if _, ok := want[slot.UTC().Unix()]; !ok {
			t.Errorf("unexpected held slot %s", slot.UTC())
		}
	}
}

// The status is under a CHECK constraint, so a typo in the domain constant would
// not be a wrong string in a column — it would be a failed insert.
func TestConsultationRepository_HeldSlots_RequestedIsAnAcceptedStatus(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewConsultationRepository(pool, testEncryptionKey)

	client, err := repo.CreateClient(ctx, "Клієнт заявки", "+380507770011", "", "")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO consultations (client_id, scheduled_at, price, status)
			VALUES (@client, now() + interval '3 days', 0, @status)`,
		pgx.NamedArgs{"client": client.ID, "status": consultations.StatusRequested}); err != nil {
		t.Fatalf("insert a requested consultation: %v", err)
	}
}
