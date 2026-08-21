//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/advocateview"
	"multiagent-seo/internal/domain/finance"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

// Two advocates with real cases each: everything below asks whether one can see
// or touch the other's. This is the whole reason the advocate login exists as a
// separate surface, so it is tested against a real database rather than a fake
// that would happily agree with whatever the SQL says.
type advocatePair struct {
	repo *postgres.AdvocateViewRepository

	borzov advocateview.Owner
	popov  advocateview.Owner

	borzovClient string
	popovClient  string
	borzovCase   string
	popovCase    string
}

func seedAdvocatePair(t *testing.T) advocatePair {
	t.Helper()
	pool := testsupport.NewTestDB(t, baseConnStr)

	may := financeDate(2026, time.May, 10)
	june := financeDate(2026, time.June, 3)

	borzovID := seedAdvocate(t, pool, "Ярослав Борзов", 35)
	popovID := seedAdvocate(t, pool, "Олег Попов", 20)

	borzovClient := seedClient(t, pool, "+380990000201", "Клієнт Борзова")
	popovClient := seedClient(t, pool, "+380990000202", "Клієнт Попова")

	borzovCase := seedCase(t, pool, borzovClient, borzovID, "Ярослав Борзов")
	popovCase := seedCase(t, pool, popovClient, popovID, "Олег Попов")

	setCaseMoney(t, pool, borzovCase, 20000, 8000, may)
	setCaseMoney(t, pool, popovCase, 15000, 15000, may)
	seedCasePayment(t, pool, borzovCase, 5000, may)
	seedCasePayment(t, pool, borzovCase, 3000, june)
	seedCasePayment(t, pool, popovCase, 15000, may)

	return advocatePair{
		repo:         postgres.NewAdvocateViewRepository(pool),
		borzov:       advocateview.Owner{ID: borzovID, FullName: "Ярослав Борзов"},
		popov:        advocateview.Owner{ID: popovID, FullName: "Олег Попов"},
		borzovClient: borzovClient,
		popovClient:  popovClient,
		borzovCase:   borzovCase,
		popovCase:    popovCase,
	}
}

// setCaseMoney fills in what seedCase leaves at zero — fee, running paid total
// and the created date.
func setCaseMoney(t *testing.T, pool *pgxpool.Pool, caseID string, fee, paid float64, createdAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE cases SET fee = @fee, paid_amount = @paid, created_at = @created_at WHERE id = @id`,
		pgx.NamedArgs{"fee": fee, "paid": paid, "created_at": createdAt, "id": caseID})
	if err != nil {
		t.Fatalf("set case money for %s: %v", caseID, err)
	}
}

func TestAdvocateViewRepository_CasesAreOwnOnly(t *testing.T) {
	p := seedAdvocatePair(t)

	got, err := p.repo.ListCases(context.Background(), p.borzov)
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("cases = %d, want exactly the one own case: %+v", len(got), got)
	}
	if got[0].ID != p.borzovCase {
		t.Fatalf("case = %q, want %q", got[0].ID, p.borzovCase)
	}
	if got[0].ClientName != "Клієнт Борзова" {
		t.Errorf("client = %q, want the own client", got[0].ClientName)
	}
	if got[0].Owed() != 12000 {
		t.Errorf("owed = %v, want 12000", got[0].Owed())
	}
	if len(got[0].Payments) != 2 {
		t.Errorf("payments = %+v, want both installments", got[0].Payments)
	}
}

func TestAdvocateViewRepository_ClientsAreOwnOnly(t *testing.T) {
	p := seedAdvocatePair(t)

	got, err := p.repo.ListClients(context.Background(), p.popov)
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if len(got) != 1 || got[0].ID != p.popovClient {
		t.Fatalf("clients = %+v, want only the own client", got)
	}
	if got[0].Cases != 1 || got[0].Fee != 15000 || got[0].Paid != 15000 {
		t.Errorf("client totals = %+v, want 1 case 15000/15000", got[0])
	}
}

// A colleague's client is answered exactly like a client that does not exist —
// a 403 here would confirm the id belongs to somebody.
func TestAdvocateViewRepository_ForeignClientIsNotFound(t *testing.T) {
	p := seedAdvocatePair(t)

	if _, err := p.repo.GetClient(context.Background(), p.borzov, p.popovClient); !errors.Is(err, advocateview.ErrNotFound) {
		t.Errorf("foreign client error = %v, want ErrNotFound", err)
	}
	if _, err := p.repo.GetClient(context.Background(), p.borzov, financeMissingID); !errors.Is(err, advocateview.ErrNotFound) {
		t.Errorf("missing client error = %v, want ErrNotFound", err)
	}
}

func TestAdvocateViewRepository_OwnClientCard(t *testing.T) {
	p := seedAdvocatePair(t)

	card, err := p.repo.GetClient(context.Background(), p.borzov, p.borzovClient)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if card.Client.ID != p.borzovClient || card.Client.Phone != "+380990000201" {
		t.Errorf("client = %+v, want the own client with a phone", card.Client)
	}
	if len(card.Cases) != 1 || card.Cases[0].ID != p.borzovCase {
		t.Errorf("cases = %+v, want the own case", card.Cases)
	}
}

func TestAdvocateViewRepository_NoteOnForeignClientWritesNothing(t *testing.T) {
	p := seedAdvocatePair(t)

	_, err := p.repo.AddNote(context.Background(), p.borzov, p.popovClient, "чужий клієнт", "user-1")
	if !errors.Is(err, advocateview.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}

	card, err := p.repo.GetClient(context.Background(), p.popov, p.popovClient)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if len(card.Notes) != 0 {
		t.Errorf("notes = %+v, want none — the refused insert must not have happened", card.Notes)
	}
}

func TestAdvocateViewRepository_NoteOnOwnClient(t *testing.T) {
	p := seedAdvocatePair(t)

	note, err := p.repo.AddNote(context.Background(), p.borzov, p.borzovClient, "подзвонив клієнту", "user-1")
	if err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if note.Text != "подзвонив клієнту" || note.CreatedBy != "user-1" {
		t.Errorf("note = %+v, want the text stamped with the login", note)
	}

	card, err := p.repo.GetClient(context.Background(), p.borzov, p.borzovClient)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if len(card.Notes) != 1 {
		t.Errorf("notes = %+v, want the one just written", card.Notes)
	}
}

func TestAdvocateViewRepository_StatusOnForeignCaseChangesNothing(t *testing.T) {
	p := seedAdvocatePair(t)

	err := p.repo.UpdateCaseStatus(context.Background(), p.borzov, p.popovCase, "completed")
	if !errors.Is(err, advocateview.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}

	got, err := p.repo.ListCases(context.Background(), p.popov)
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	if got[0].Status != "in_progress" {
		t.Errorf("status = %q, want it untouched", got[0].Status)
	}
}

func TestAdvocateViewRepository_StatusOnOwnCase(t *testing.T) {
	p := seedAdvocatePair(t)

	if err := p.repo.UpdateCaseStatus(context.Background(), p.borzov, p.borzovCase, "completed"); err != nil {
		t.Fatalf("UpdateCaseStatus: %v", err)
	}
	got, err := p.repo.ListCases(context.Background(), p.borzov)
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	if got[0].Status != "completed" {
		t.Errorf("status = %q, want completed", got[0].Status)
	}
}

func TestAdvocateViewRepository_CollectionsAreOwnOnly(t *testing.T) {
	p := seedAdvocatePair(t)

	got, err := p.repo.CollectionsByMonth(context.Background(), p.borzov)
	if err != nil {
		t.Fatalf("CollectionsByMonth: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("months = %+v, want May and June", got)
	}
	if got[0].Month != "2026-05" || got[0].Collected != 5000 {
		t.Errorf("may = %+v, want 5000 — the colleague's 15000 must not be in it", got[0])
	}
	if got[1].Month != "2026-06" || got[1].Collected != 3000 {
		t.Errorf("june = %+v, want 3000", got[1])
	}
}

// A case from before the roster carries only a surname. The advocate view has
// to see it (it is their work and their money) and the finance settlement has
// to attribute the same payment to the same person — if these two ever disagree,
// an advocate's page and their payout stop matching.
func TestAdvocateViewRepository_LegacyCaseByNameAgreesWithFinance(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)

	may := financeDate(2026, time.May, 10)
	advocateID := seedAdvocate(t, pool, "Ярослав Борзов", 35)
	clientID := seedClient(t, pool, "+380990000203", "Старий клієнт")
	legacyCase := seedCase(t, pool, clientID, "", "Ярослав Борзов")
	setCaseMoney(t, pool, legacyCase, 10000, 4000, may)
	seedCasePayment(t, pool, legacyCase, 4000, may)

	owner := advocateview.Owner{ID: advocateID, FullName: "Ярослав Борзов"}
	view := postgres.NewAdvocateViewRepository(pool)

	cases, err := view.ListCases(ctx, owner)
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	if len(cases) != 1 || cases[0].ID != legacyCase {
		t.Fatalf("cases = %+v, want the unlinked case matched by name", cases)
	}

	months, err := view.CollectionsByMonth(ctx, owner)
	if err != nil {
		t.Fatalf("CollectionsByMonth: %v", err)
	}
	collections, err := postgres.NewFinanceRepository(pool).
		AdvocateCollections(ctx, financeDate(2026, time.May, 1), financeDate(2026, time.May, 31))
	if err != nil {
		t.Fatalf("AdvocateCollections: %v", err)
	}
	if len(months) != 1 || len(collections) != 1 {
		t.Fatalf("view months = %+v, finance rows = %+v, want one each", months, collections)
	}
	if months[0].Collected != collections[0].Collected {
		t.Errorf("the advocate's own page says %v collected, the settlement says %v",
			months[0].Collected, collections[0].Collected)
	}
}

// An advocate whose roster name is blank must not inherit every case that also
// has a blank advocate_name — the legacy name fallback has to stay a match on a
// real name.
func TestAdvocateViewRepository_BlankNameOwnsNothingByName(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)

	advocateID := seedAdvocate(t, pool, "", 0)
	clientID := seedClient(t, pool, "+380990000204", "Нічий клієнт")
	seedCase(t, pool, clientID, "", "")

	got, err := postgres.NewAdvocateViewRepository(pool).
		ListCases(ctx, advocateview.Owner{ID: advocateID, FullName: ""})
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("cases = %+v, want none", got)
	}
}

func TestAdvocateViewRepository_PaidOutCountsOnlyOwnPayouts(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)

	repo := postgres.NewFinanceRepository(pool)
	mine := seedAdvocate(t, pool, "Ярослав Борзов", 35)
	theirs := seedAdvocate(t, pool, "Олег Попов", 20)

	postPayout(t, repo, "advocate:"+mine+":2026-05", 3000)
	postPayout(t, repo, "advocate:"+theirs+":2026-05", 9000)
	postPayout(t, repo, "", 7000) // an imported lump sum that names nobody

	got, err := postgres.NewAdvocateViewRepository(pool).PaidOut(ctx, mine)
	if err != nil {
		t.Fatalf("PaidOut: %v", err)
	}
	if got != 3000 {
		t.Errorf("paid out = %v, want 3000 — neither the colleague's payout nor the unattributed lump sum", got)
	}
}

// postPayout books a confirmed payout in the advocates category — the shape
// PaidOut reads.
func postPayout(t *testing.T, repo *postgres.FinanceRepository, externalRef string, amount float64) {
	t.Helper()
	createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.May, 31),
		Amount:       amount,
		CategoryCode: "advocates",
		Status:       finance.StatusPosted,
		Origin:       finance.OriginRecurring,
		ExternalRef:  externalRef,
	})
}
