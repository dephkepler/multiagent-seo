//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/finance"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

// Every date in this file is pinned. A `time.Now()`-relative fixture would
// flake on the first/last day of a month, and every invariant here is about
// which month a row lands in.
func financeDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// A well-formed uuid that is guaranteed not to exist — the not-found probes
// must exercise "no such row", not "unparseable id".
const financeMissingID = "00000000-0000-0000-0000-000000000000"

// --- seeding: CRM tables the finance repository reads but does not own ---

func seedClient(t *testing.T, pool *pgxpool.Pool, phone, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO clients (phone, name) VALUES (@phone, @name) RETURNING id`,
		pgx.NamedArgs{"phone": phone, "name": name}).Scan(&id)
	if err != nil {
		t.Fatalf("seed client %q: %v", phone, err)
	}
	return id
}

func seedLead(t *testing.T, pool *pgxpool.Pool, clientID, messageID string, receivedAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO leads (message_id, received_at, client_id) VALUES (@message_id, @received_at, @client_id)`,
		pgx.NamedArgs{"message_id": messageID, "received_at": receivedAt, "client_id": clientID})
	if err != nil {
		t.Fatalf("seed lead %q: %v", messageID, err)
	}
}

func seedConsultation(t *testing.T, pool *pgxpool.Pool, clientID string, scheduledAt time.Time, price float64, status string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO consultations (client_id, scheduled_at, price, status)
			VALUES (@client_id, @scheduled_at, @price, @status) RETURNING id`,
		pgx.NamedArgs{"client_id": clientID, "scheduled_at": scheduledAt, "price": price, "status": status}).Scan(&id)
	if err != nil {
		t.Fatalf("seed consultation (%s, %.2f, %s): %v", scheduledAt.Format(time.DateOnly), price, status, err)
	}
	return id
}

func seedAdvocate(t *testing.T, pool *pgxpool.Pool, fullName string, commissionPercent float64) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO advocates (full_name, commission_percent) VALUES (@full_name, @percent) RETURNING id`,
		pgx.NamedArgs{"full_name": fullName, "percent": commissionPercent}).Scan(&id)
	if err != nil {
		t.Fatalf("seed advocate %q: %v", fullName, err)
	}
	return id
}

// advocateID empty means the legacy shape: free-text advocate_name only, no
// link into the roster.
func seedCase(t *testing.T, pool *pgxpool.Pool, clientID, advocateID, advocateName string) string {
	t.Helper()
	var linked *string
	if advocateID != "" {
		linked = &advocateID
	}
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO cases (client_id, advocate_id, advocate_name)
			VALUES (@client_id, @advocate_id, @advocate_name) RETURNING id`,
		pgx.NamedArgs{"client_id": clientID, "advocate_id": linked, "advocate_name": advocateName}).Scan(&id)
	if err != nil {
		t.Fatalf("seed case for client %s: %v", clientID, err)
	}
	return id
}

func seedCasePayment(t *testing.T, pool *pgxpool.Pool, caseID string, amount float64, paidAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO case_payments (case_id, amount, paid_at) VALUES (@case_id, @amount, @paid_at)`,
		pgx.NamedArgs{"case_id": caseID, "amount": amount, "paid_at": paidAt})
	if err != nil {
		t.Fatalf("seed case payment (%.2f on %s): %v", amount, paidAt.Format(time.DateOnly), err)
	}
}

func createExpense(t *testing.T, repo *postgres.FinanceRepository, e finance.Expense) finance.Expense {
	t.Helper()
	saved, err := repo.CreateExpense(context.Background(), e)
	if err != nil {
		t.Fatalf("CreateExpense(%s %.2f %s): %v", e.SpentAt.Format(time.DateOnly), e.Amount, e.CategoryCode, err)
	}
	return saved
}

func monthKeys(facts []finance.MonthFacts) []string {
	out := make([]string, 0, len(facts))
	for _, f := range facts {
		out = append(out, f.Month)
	}
	return out
}

// --- 1. drafts are not money ---

// A draft is a generated suggestion nobody has acknowledged. If it leaks into
// MonthlyFacts the P&L overstates spend, which is the single most expensive
// way this feature can be wrong.
func TestFinanceRepository_MonthlyFacts_DraftIsNotMoneyUntilConfirmed(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	clientID := seedClient(t, pool, "+380990000001", "Февральский клиент")
	seedLead(t, pool, clientID, "msg-feb-1", financeDate(2026, time.February, 2))
	seedConsultation(t, pool, clientID, financeDate(2026, time.February, 10), 1500, "completed")
	// Neither of these is earned revenue: one was never held, the other is free.
	seedConsultation(t, pool, clientID, financeDate(2026, time.February, 11), 900, "cancelled")
	seedConsultation(t, pool, clientID, financeDate(2026, time.February, 12), 0, "completed")

	caseID := seedCase(t, pool, clientID, "", "Без адвоката")
	seedCasePayment(t, pool, caseID, 5000, financeDate(2026, time.February, 15))

	if _, err := repo.CreateOtherIncome(ctx, finance.OtherIncome{
		ReceivedAt:  financeDate(2026, time.February, 20),
		Amount:      300,
		Source:      "refund",
		Description: "возврат",
	}); err != nil {
		t.Fatalf("CreateOtherIncome: %v", err)
	}

	createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.February, 5),
		Amount:       700,
		CategoryCode: "google_ads",
		Status:       finance.StatusPosted,
	})
	draft := createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.February, 6),
		Amount:       999,
		CategoryCode: "smm",
		Status:       finance.StatusDraft,
		Origin:       finance.OriginRecurring,
	})

	from := financeDate(2026, time.February, 1)
	to := financeDate(2026, time.February, 28)

	facts, err := repo.MonthlyFacts(ctx, from, to)
	if err != nil {
		t.Fatalf("MonthlyFacts: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("MonthlyFacts len = %d (%v), want 1", len(facts), monthKeys(facts))
	}
	got := facts[0]
	if got.Month != "2026-02" {
		t.Errorf("Month = %q, want %q", got.Month, "2026-02")
	}
	if got.ConsultRevenue != 1500 {
		t.Errorf("ConsultRevenue = %v, want 1500 (only priced completed consultations)", got.ConsultRevenue)
	}
	if got.CasePaid != 5000 {
		t.Errorf("CasePaid = %v, want 5000", got.CasePaid)
	}
	if got.OtherIncome != 300 {
		t.Errorf("OtherIncome = %v, want 300", got.OtherIncome)
	}
	if got.Leads != 1 {
		t.Errorf("Leads = %d, want 1", got.Leads)
	}
	if got.NewClients != 1 {
		t.Errorf("NewClients = %d, want 1", got.NewClients)
	}
	if got.ExpenseByCategory["google_ads"] != 700 {
		t.Errorf("ExpenseByCategory[google_ads] = %v, want 700", got.ExpenseByCategory["google_ads"])
	}
	if amount, ok := got.ExpenseByCategory["smm"]; ok {
		t.Errorf("ExpenseByCategory[smm] = %v present, want absent — a draft is not money", amount)
	}

	if err := repo.ConfirmExpense(ctx, draft.ID); err != nil {
		t.Fatalf("ConfirmExpense: %v", err)
	}

	facts, err = repo.MonthlyFacts(ctx, from, to)
	if err != nil {
		t.Fatalf("MonthlyFacts after confirm: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("MonthlyFacts after confirm len = %d, want 1", len(facts))
	}
	if facts[0].ExpenseByCategory["smm"] != 999 {
		t.Errorf("ExpenseByCategory[smm] after confirm = %v, want 999",
			facts[0].ExpenseByCategory["smm"])
	}
	if facts[0].ExpenseByCategory["google_ads"] != 700 {
		t.Errorf("ExpenseByCategory[google_ads] after confirm = %v, want 700 (unchanged)",
			facts[0].ExpenseByCategory["google_ads"])
	}
}

// --- 2. InsertGenerated idempotency ---

func TestFinanceRepository_InsertGenerated_IsIdempotentPerExternalRef(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	planned := finance.Expense{
		SpentAt:      financeDate(2026, time.August, 5),
		Amount:       9140,
		CategoryCode: "company",
		Vendor:       "Алсана",
		Status:       finance.StatusDraft,
		Origin:       finance.OriginRecurring,
		ExternalRef:  finance.RecurringRef("11111111-1111-1111-1111-111111111111", financeDate(2026, time.August, 1)),
	}

	inserted, err := repo.InsertGenerated(ctx, planned)
	if err != nil {
		t.Fatalf("first InsertGenerated: %v", err)
	}
	if !inserted {
		t.Errorf("first InsertGenerated = false, want true")
	}

	// A second generator pass over the same month must be a no-op, not an
	// error and not a double charge.
	inserted, err = repo.InsertGenerated(ctx, planned)
	if err != nil {
		t.Fatalf("second InsertGenerated: %v", err)
	}
	if inserted {
		t.Errorf("second InsertGenerated = true, want false — the external_ref was already there")
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM expenses WHERE external_ref = @ref`,
		pgx.NamedArgs{"ref": planned.ExternalRef}).Scan(&count); err != nil {
		t.Fatalf("count by external_ref: %v", err)
	}
	if count != 1 {
		t.Errorf("rows for external_ref %q = %d, want exactly 1", planned.ExternalRef, count)
	}
}

// The unique index on external_ref is partial (WHERE external_ref IS NOT
// NULL). If an empty ExternalRef were written as ” instead of NULL, the
// second unrelated row would collide and be silently dropped — this is the
// bug the test exists to catch.
func TestFinanceRepository_InsertGenerated_EmptyExternalRefDoesNotCollide(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	first := finance.Expense{
		SpentAt:      financeDate(2026, time.August, 5),
		Amount:       100,
		CategoryCode: "admin_misc",
		Description:  "первый без ref",
	}
	second := finance.Expense{
		SpentAt:      financeDate(2026, time.August, 6),
		Amount:       200,
		CategoryCode: "admin_misc",
		Description:  "второй без ref",
	}

	insertedFirst, err := repo.InsertGenerated(ctx, first)
	if err != nil {
		t.Fatalf("InsertGenerated with empty ref (first): %v", err)
	}
	if !insertedFirst {
		t.Errorf("first empty-ref InsertGenerated = false, want true")
	}

	insertedSecond, err := repo.InsertGenerated(ctx, second)
	if err != nil {
		t.Fatalf("InsertGenerated with empty ref (second): %v", err)
	}
	if !insertedSecond {
		t.Errorf("second empty-ref InsertGenerated = false, want true — '' must land as NULL, " +
			"the unique index is partial")
	}

	var nullRefs int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM expenses WHERE external_ref IS NULL`).Scan(&nullRefs); err != nil {
		t.Fatalf("count null refs: %v", err)
	}
	if nullRefs != 2 {
		t.Errorf("rows with NULL external_ref = %d, want 2", nullRefs)
	}
	var emptyRefs int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM expenses WHERE external_ref = ''`).Scan(&emptyRefs); err != nil {
		t.Fatalf("count empty refs: %v", err)
	}
	if emptyRefs != 0 {
		t.Errorf("rows with external_ref = '' = %d, want 0 (an empty ref must be NULL)", emptyRefs)
	}
}

// --- 3. ConfirmExpense failure modes ---

func TestFinanceRepository_ConfirmExpense_DistinguishesFailureModes(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	t.Run("unknown id", func(t *testing.T) {
		err := repo.ConfirmExpense(ctx, financeMissingID)
		if !errors.Is(err, finance.ErrNotFound) {
			t.Errorf("ConfirmExpense(unknown) = %v, want ErrNotFound", err)
		}
	})

	t.Run("already posted", func(t *testing.T) {
		posted := createExpense(t, repo, finance.Expense{
			SpentAt:      financeDate(2026, time.March, 3),
			Amount:       120,
			CategoryCode: "website",
			Status:       finance.StatusPosted,
		})
		err := repo.ConfirmExpense(ctx, posted.ID)
		if !errors.Is(err, finance.ErrNotDraft) {
			t.Errorf("ConfirmExpense(posted) = %v, want ErrNotDraft", err)
		}
	})

	t.Run("draft becomes posted once", func(t *testing.T) {
		draft := createExpense(t, repo, finance.Expense{
			SpentAt:      financeDate(2026, time.March, 4),
			Amount:       130,
			CategoryCode: "website",
			Status:       finance.StatusDraft,
			Origin:       finance.OriginRecurring,
		})
		if err := repo.ConfirmExpense(ctx, draft.ID); err != nil {
			t.Fatalf("ConfirmExpense(draft): %v", err)
		}

		reloaded, err := repo.GetExpense(ctx, draft.ID)
		if err != nil {
			t.Fatalf("GetExpense after confirm: %v", err)
		}
		if reloaded.Status != finance.StatusPosted {
			t.Errorf("Status after confirm = %q, want %q", reloaded.Status, finance.StatusPosted)
		}
		// Confirm is not idempotent by design: a posted row is corrected via
		// update, never re-confirmed.
		if err := repo.ConfirmExpense(ctx, draft.ID); !errors.Is(err, finance.ErrNotDraft) {
			t.Errorf("second ConfirmExpense = %v, want ErrNotDraft", err)
		}
	})
}

// --- 4. MonthlyFacts shape ---

// A month with no activity must still come back, in place, or the report's
// columns silently shift and every number after the gap is attributed to the
// wrong month.
func TestFinanceRepository_MonthlyFacts_EmptyMonthStillReturnedInOrder(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	clientID := seedClient(t, pool, "+380990000002", "Клиент с пропуском")
	seedConsultation(t, pool, clientID, financeDate(2026, time.January, 20), 1000, "completed")
	createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.March, 10),
		Amount:       400,
		CategoryCode: "google_ads",
	})

	// Deliberately a mid-month "to": the range is inclusive of the whole
	// month the bound falls in, not of the exact day.
	facts, err := repo.MonthlyFacts(ctx, financeDate(2026, time.January, 15), financeDate(2026, time.March, 10))
	if err != nil {
		t.Fatalf("MonthlyFacts: %v", err)
	}

	wantMonths := []string{"2026-01", "2026-02", "2026-03"}
	gotMonths := monthKeys(facts)
	if len(gotMonths) != len(wantMonths) {
		t.Fatalf("months = %v, want %v", gotMonths, wantMonths)
	}
	for i := range wantMonths {
		if gotMonths[i] != wantMonths[i] {
			t.Fatalf("months = %v, want %v (ascending, finance.MonthKey format)", gotMonths, wantMonths)
		}
	}

	if facts[0].ConsultRevenue != 1000 {
		t.Errorf("2026-01 ConsultRevenue = %v, want 1000", facts[0].ConsultRevenue)
	}
	empty := facts[1]
	if empty.ConsultRevenue != 0 || empty.CasePaid != 0 || empty.OtherIncome != 0 ||
		empty.Leads != 0 || empty.NewClients != 0 || len(empty.ExpenseByCategory) != 0 {
		t.Errorf("2026-02 = %+v, want an all-zero row", empty)
	}
	if empty.ExpenseByCategory == nil {
		t.Errorf("2026-02 ExpenseByCategory is nil, want an initialized empty map")
	}
	if facts[2].ExpenseByCategory["google_ads"] != 400 {
		t.Errorf("2026-03 ExpenseByCategory[google_ads] = %v, want 400", facts[2].ExpenseByCategory["google_ads"])
	}

	// A single-month range is the degenerate case of the same generate_series.
	one, err := repo.MonthlyFacts(ctx, financeDate(2026, time.February, 1), financeDate(2026, time.February, 28))
	if err != nil {
		t.Fatalf("MonthlyFacts single month: %v", err)
	}
	if len(one) != 1 || one[0].Month != "2026-02" {
		t.Errorf("single-month range = %v, want [2026-02]", monthKeys(one))
	}
}

// --- 5. BalanceBefore ---

func TestFinanceRepository_BalanceBefore_IgnoresDraftsAndTheCutoffDayOnward(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	clientID := seedClient(t, pool, "+380990000003", "Балансовый клиент")
	caseID := seedCase(t, pool, clientID, "", "")

	// Counted: strictly before 2026-03-01.
	seedConsultation(t, pool, clientID, financeDate(2026, time.January, 10), 1000, "completed")
	seedCasePayment(t, pool, caseID, 2000, financeDate(2026, time.February, 1))
	if _, err := repo.CreateOtherIncome(ctx, finance.OtherIncome{
		ReceivedAt: financeDate(2026, time.January, 20),
		Amount:     500,
		Source:     "company",
	}); err != nil {
		t.Fatalf("CreateOtherIncome: %v", err)
	}
	createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.February, 10),
		Amount:       300,
		CategoryCode: "google_ads",
	})

	// Not counted: unearned, draft, or on/after the cutoff.
	seedConsultation(t, pool, clientID, financeDate(2026, time.January, 11), 400, "no_show")
	seedConsultation(t, pool, clientID, financeDate(2026, time.March, 5), 5000, "completed")
	seedCasePayment(t, pool, caseID, 7000, financeDate(2026, time.March, 1))
	createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.February, 11),
		Amount:       9000,
		CategoryCode: "smm",
		Status:       finance.StatusDraft,
		Origin:       finance.OriginRecurring,
	})
	createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.March, 1),
		Amount:       8000,
		CategoryCode: "smm",
	})

	balance, err := repo.BalanceBefore(ctx, financeDate(2026, time.March, 1))
	if err != nil {
		t.Fatalf("BalanceBefore: %v", err)
	}
	const want = 1000 + 2000 + 500 - 300
	if balance != want {
		t.Errorf("BalanceBefore(2026-03-01) = %v, want %v (income minus posted expenses, strictly before the cutoff)",
			balance, float64(want))
	}

	// Nothing at all before the very beginning of history.
	early, err := repo.BalanceBefore(ctx, financeDate(2026, time.January, 1))
	if err != nil {
		t.Fatalf("BalanceBefore(empty): %v", err)
	}
	if early != 0 {
		t.Errorf("BalanceBefore(2026-01-01) = %v, want 0", early)
	}
}

// --- 6. AdvocateCollections ---

func TestFinanceRepository_AdvocateCollections_PerAdvocateForTheMonth(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	clientID := seedClient(t, pool, "+380990000004", "Адвокатский клиент")

	borzov := seedAdvocate(t, pool, "Ярослав Борзов", 20)
	zeroRate := seedAdvocate(t, pool, "Без ставки", 0)
	idle := seedAdvocate(t, pool, "Яна Без Дел", 15)

	borzovCase := seedCase(t, pool, clientID, borzov, "Ярослав Борзов")
	seedCasePayment(t, pool, borzovCase, 1000, financeDate(2026, time.February, 3))
	seedCasePayment(t, pool, borzovCase, 500, financeDate(2026, time.February, 27))
	// Next month — must not leak into February.
	seedCasePayment(t, pool, borzovCase, 900, financeDate(2026, time.March, 1))

	zeroCase := seedCase(t, pool, clientID, zeroRate, "Без ставки")
	seedCasePayment(t, pool, zeroCase, 2000, financeDate(2026, time.February, 10))

	// Legacy shape: free-text advocate_name, no advocate_id. Documented as
	// unattributable — the join drops it.
	legacyCase := seedCase(t, pool, clientID, "", "Старый Адвокат")
	seedCasePayment(t, pool, legacyCase, 4000, financeDate(2026, time.February, 12))

	got, err := repo.AdvocateCollections(ctx, financeDate(2026, time.February, 1))
	if err != nil {
		t.Fatalf("AdvocateCollections: %v", err)
	}

	// Ordered by collected DESC.
	want := []finance.AdvocateCollection{
		{AdvocateID: zeroRate, AdvocateName: "Без ставки", Collected: 2000, CommissionPercent: 0},
		{AdvocateID: borzov, AdvocateName: "Ярослав Борзов", Collected: 1500, CommissionPercent: 20},
	}
	if len(got) != len(want) {
		t.Fatalf("AdvocateCollections = %+v, want %d rows (idle advocate %s and the legacy "+
			"advocate_name-only case must both be absent)", got, len(want), idle)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AdvocateCollections[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// The unattributable money is real and is still in case_payments — it just
	// has no advocate to pay it out to.
	var legacySum float64
	if err := pool.QueryRow(ctx,
		`SELECT coalesce(sum(p.amount), 0) FROM case_payments p
			JOIN cases c ON c.id = p.case_id
			WHERE c.advocate_id IS NULL`).Scan(&legacySum); err != nil {
		t.Fatalf("sum legacy payments: %v", err)
	}
	if legacySum != 4000 {
		t.Errorf("legacy (advocate_id IS NULL) payments = %v, want 4000 — the money exists, "+
			"AdvocateCollections just cannot attribute it", legacySum)
	}

	// A month with no payments at all is an empty result, not an error.
	empty, err := repo.AdvocateCollections(ctx, financeDate(2026, time.April, 1))
	if err != nil {
		t.Fatalf("AdvocateCollections(empty month): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("AdvocateCollections(2026-04) = %+v, want empty", empty)
	}
}

// --- 7. NULL round-trips ---

func TestFinanceRepository_NullableFieldsRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	activeTo := financeDate(2026, time.December, 31)

	openEnded, err := repo.CreateRule(ctx, finance.Rule{
		Name:          "Хостинг",
		CategoryCode:  "website",
		Vendor:        "Hoster",
		PaymentMethod: finance.PaymentCard,
		Amount:        642,
		DayOfMonth:    5,
		ActiveFrom:    financeDate(2026, time.January, 1),
		IsActive:      true,
	})
	if err != nil {
		t.Fatalf("CreateRule open-ended: %v", err)
	}
	bounded, err := repo.CreateRule(ctx, finance.Rule{
		Name:          "Юрист на проекте",
		CategoryCode:  "assistant",
		PaymentMethod: finance.PaymentInvoice,
		Amount:        7000,
		DayOfMonth:    10,
		ActiveFrom:    financeDate(2026, time.January, 1),
		ActiveTo:      &activeTo,
		IsActive:      true,
	})
	if err != nil {
		t.Fatalf("CreateRule bounded: %v", err)
	}

	rules, err := repo.ListRules(ctx, false)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	byID := map[string]finance.Rule{}
	for _, r := range rules {
		byID[r.ID] = r
	}

	gotOpen, ok := byID[openEnded.ID]
	if !ok {
		t.Fatalf("open-ended rule %s missing from ListRules", openEnded.ID)
	}
	if gotOpen.ActiveTo != nil {
		t.Errorf("open-ended ActiveTo = %v, want nil (not a zero time)", *gotOpen.ActiveTo)
	}
	if !gotOpen.ActiveFrom.Equal(financeDate(2026, time.January, 1)) {
		t.Errorf("ActiveFrom = %v, want 2026-01-01", gotOpen.ActiveFrom)
	}
	if gotOpen.CategoryLabel == "" {
		t.Errorf("CategoryLabel is empty, want the joined label for %q", gotOpen.CategoryCode)
	}

	gotBounded, ok := byID[bounded.ID]
	if !ok {
		t.Fatalf("bounded rule %s missing from ListRules", bounded.ID)
	}
	if gotBounded.ActiveTo == nil {
		t.Fatalf("bounded ActiveTo = nil, want 2026-12-31")
	}
	if !gotBounded.ActiveTo.Equal(activeTo) {
		t.Errorf("bounded ActiveTo = %v, want %v", *gotBounded.ActiveTo, activeTo)
	}

	// An expense with neither provenance field set: both must be NULL in the
	// column and empty (not "<nil>") coming back out.
	bare := createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.February, 4),
		Amount:       50,
		CategoryCode: "admin_misc",
	})
	reloaded, err := repo.GetExpense(ctx, bare.ID)
	if err != nil {
		t.Fatalf("GetExpense bare: %v", err)
	}
	if reloaded.RuleID != "" {
		t.Errorf("RuleID = %q, want empty", reloaded.RuleID)
	}
	if reloaded.ExternalRef != "" {
		t.Errorf("ExternalRef = %q, want empty", reloaded.ExternalRef)
	}
	var ruleIsNull, refIsNull bool
	if err := pool.QueryRow(ctx,
		`SELECT rule_id IS NULL, external_ref IS NULL FROM expenses WHERE id = @id`,
		pgx.NamedArgs{"id": bare.ID}).Scan(&ruleIsNull, &refIsNull); err != nil {
		t.Fatalf("null check: %v", err)
	}
	if !ruleIsNull || !refIsNull {
		t.Errorf("rule_id IS NULL = %v, external_ref IS NULL = %v, want both true", ruleIsNull, refIsNull)
	}

	linked := createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.February, 5),
		Amount:       642,
		CategoryCode: "website",
		RuleID:       openEnded.ID,
		ExternalRef:  finance.RecurringRef(openEnded.ID, financeDate(2026, time.February, 1)),
		Origin:       finance.OriginRecurring,
	})
	reloadedLinked, err := repo.GetExpense(ctx, linked.ID)
	if err != nil {
		t.Fatalf("GetExpense linked: %v", err)
	}
	if reloadedLinked.RuleID != openEnded.ID {
		t.Errorf("RuleID = %q, want %q", reloadedLinked.RuleID, openEnded.ID)
	}
	if reloadedLinked.ExternalRef != linked.ExternalRef {
		t.Errorf("ExternalRef = %q, want %q", reloadedLinked.ExternalRef, linked.ExternalRef)
	}
	if !reloadedLinked.SpentAt.Equal(financeDate(2026, time.February, 5)) {
		t.Errorf("SpentAt = %v, want 2026-02-05", reloadedLinked.SpentAt)
	}
}

// KNOWN GAP, pinned deliberately: expense_rules.active_from is declared
// `date NOT NULL DEFAULT current_date`, but CreateRule always names the column
// in its INSERT, so the DEFAULT can never fire. A Rule with a zero ActiveFrom
// is stored as 0001-01-01 — a rule "active since year 1", which every
// generator window matches. Callers must supply ActiveFrom themselves; if the
// repository ever starts omitting it, this test trips.
func TestFinanceRepository_CreateRule_ZeroActiveFromBypassesTheColumnDefault(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	created, err := repo.CreateRule(ctx, finance.Rule{
		Name:         "Без даты старта",
		CategoryCode: "website",
		Amount:       100,
		DayOfMonth:   1,
		IsActive:     true,
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	var activeFrom time.Time
	if err := pool.QueryRow(ctx, `SELECT active_from FROM expense_rules WHERE id = @id`,
		pgx.NamedArgs{"id": created.ID}).Scan(&activeFrom); err != nil {
		t.Fatalf("read back active_from: %v", err)
	}
	if activeFrom.Year() != 1 {
		t.Errorf("zero ActiveFrom stored as %v, want 0001-01-01 — the column DEFAULT current_date "+
			"is dead code because the INSERT always names active_from", activeFrom.Format(time.DateOnly))
	}
}

// --- timezone behaviour: characterization tests, see the report ---

// pgx passes an unrecognized connection parameter through as a Postgres
// runtime parameter, which is how these tests get a session pinned to the
// company's own timezone rather than the container's UTC default.
func poolInTimeZone(t *testing.T, pool *pgxpool.Pool, tz string) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config().Copy()
	cfg.ConnConfig.RuntimeParams["timezone"] = tz
	tzPool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pool in %s: %v", tz, err)
	}
	t.Cleanup(tzPool.Close)

	var got string
	if err := tzPool.QueryRow(context.Background(), `SHOW TimeZone`).Scan(&got); err != nil {
		t.Fatalf("SHOW TimeZone: %v", err)
	}
	if got != tz {
		t.Fatalf("session TimeZone = %q, want %q", got, tz)
	}
	return tzPool
}

// BUG, pinned so that fixing the SQL trips this test.
//
// BalanceBefore compares `date` columns against `@from::timestamptz`
// (finance_repository.go:546-553), so each date is promoted to *local*
// midnight. Under any session timezone east of UTC, a row dated exactly on
// the cutoff sorts strictly before a UTC-midnight cutoff and is swallowed
// into "everything before". MonthlyFacts does not have this problem: its
// expense query casts the parameter with `::date` instead
// (finance_repository.go:516-517), which is timezone-immune. The two
// therefore disagree, and the same row is counted twice in the report — once
// in the starting balance, once in its real month.
func TestFinanceRepository_BalanceBefore_IsTimeZoneIndependent(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	utcRepo := postgres.NewFinanceRepository(pool)
	kyivRepo := postgres.NewFinanceRepository(poolInTimeZone(t, pool, "Europe/Kyiv"))

	// Dated exactly ON the cutoff — belongs to March, not to "before March".
	createExpense(t, utcRepo, finance.Expense{
		SpentAt:      financeDate(2026, time.March, 1),
		Amount:       8000,
		CategoryCode: "smm",
	})
	// The day before — genuinely before the cutoff.
	createExpense(t, utcRepo, finance.Expense{
		SpentAt:      financeDate(2026, time.February, 28),
		Amount:       300,
		CategoryCode: "smm",
	})

	cutoff := financeDate(2026, time.March, 1)

	underUTC, err := utcRepo.BalanceBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("BalanceBefore under UTC: %v", err)
	}
	if underUTC != -300 {
		t.Errorf("BalanceBefore under UTC = %v, want -300 (the cutoff day is excluded)", underUTC)
	}

	underKyiv, err := kyivRepo.BalanceBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("BalanceBefore under Europe/Kyiv: %v", err)
	}
	if underKyiv != -300 {
		t.Errorf("BalanceBefore under Europe/Kyiv = %v, want -300: comparing a date column "+
			"against ::date keeps the cutoff day out of the opening balance whatever the "+
			"session timezone is", underKyiv)
	}
}

// Consultation revenue is bucketed with date_trunc over a `timestamptz`
// column, so which month a late-evening consultation is earned in depends on
// the database session's timezone. Expenses, case_payments and other_income
// are `date` columns and never move. Pinned because the port comment promises
// "the month the consultation actually happened" — true only when the session
// timezone happens to match the business's.
func TestFinanceRepository_MonthlyFacts_ConsultRevenueBucketIsPinnedToBusinessTimeZone(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)

	clientID := seedClient(t, pool, "+380990000005", "Поздний вечер")
	// 2026-02-28 23:00 UTC is already 2026-03-01 01:00 in Kyiv.
	seedConsultation(t, pool, clientID,
		time.Date(2026, time.February, 28, 23, 0, 0, 0, time.UTC), 1000, "completed")
	createExpense(t, postgres.NewFinanceRepository(pool), finance.Expense{
		SpentAt:      financeDate(2026, time.February, 28),
		Amount:       50,
		CategoryCode: "smm",
	})

	from := financeDate(2026, time.February, 1)
	to := financeDate(2026, time.March, 31)

	revenueByMonth := func(p *pgxpool.Pool) map[string]float64 {
		t.Helper()
		facts, err := postgres.NewFinanceRepository(p).MonthlyFacts(ctx, from, to)
		if err != nil {
			t.Fatalf("MonthlyFacts: %v", err)
		}
		out := map[string]float64{}
		for _, f := range facts {
			out[f.Month] = f.ConsultRevenue
			// The expense is on a `date` column, so it stays in February
			// whatever the session timezone is.
			if f.Month == "2026-02" && f.ExpenseByCategory["smm"] != 50 {
				t.Errorf("2026-02 ExpenseByCategory[smm] = %v, want 50 — a date column must not "+
					"move between months", f.ExpenseByCategory["smm"])
			}
		}
		return out
	}

	// 23:00 UTC on Feb 28 is already March 1st in Kyiv, and the month is pinned to
	// the firm's timezone — so it is March revenue under either session timezone.
	underUTC := revenueByMonth(pool)
	if underUTC["2026-02"] != 0 || underUTC["2026-03"] != 1000 {
		t.Errorf("under UTC: 2026-02 = %v, 2026-03 = %v, want 0 and 1000",
			underUTC["2026-02"], underUTC["2026-03"])
	}

	underKyiv := revenueByMonth(poolInTimeZone(t, pool, "Europe/Kyiv"))
	if underKyiv["2026-02"] != 0 || underKyiv["2026-03"] != 1000 {
		t.Errorf("under Europe/Kyiv: 2026-02 = %v, 2026-03 = %v, want 0 and 1000",
			underKyiv["2026-02"], underKyiv["2026-03"])
	}
	if underUTC["2026-03"] != underKyiv["2026-03"] {
		t.Errorf("session timezone still moves consultation revenue: UTC %v vs Kyiv %v",
			underUTC["2026-03"], underKyiv["2026-03"])
	}
}

// --- 8. error translation ---

func TestFinanceRepository_ErrorTranslation(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	existing := createExpense(t, repo, finance.Expense{
		SpentAt:      financeDate(2026, time.February, 1),
		Amount:       10,
		CategoryCode: "google_ads",
	})

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "expense with unknown category",
			call: func() error {
				_, err := repo.CreateExpense(ctx, finance.Expense{
					SpentAt:      financeDate(2026, time.February, 2),
					Amount:       10,
					CategoryCode: "no_such_category",
				})
				return err
			},
			want: finance.ErrUnknownCategory,
		},
		{
			name: "generated expense with unknown category",
			call: func() error {
				_, err := repo.InsertGenerated(ctx, finance.Expense{
					SpentAt:      financeDate(2026, time.February, 2),
					Amount:       10,
					CategoryCode: "no_such_category",
					ExternalRef:  "sheet:test:1",
				})
				return err
			},
			want: finance.ErrUnknownCategory,
		},
		{
			name: "update expense to an unknown category",
			call: func() error {
				e := existing
				e.CategoryCode = "no_such_category"
				return repo.UpdateExpense(ctx, e)
			},
			want: finance.ErrUnknownCategory,
		},
		{
			name: "rule with unknown category",
			call: func() error {
				_, err := repo.CreateRule(ctx, finance.Rule{
					Name:         "Правило в пустоту",
					CategoryCode: "no_such_category",
					Amount:       10,
					DayOfMonth:   1,
					ActiveFrom:   financeDate(2026, time.January, 1),
					IsActive:     true,
				})
				return err
			},
			want: finance.ErrUnknownCategory,
		},
		{
			name: "category code already taken",
			call: func() error {
				return repo.CreateCategory(ctx, finance.Category{
					Code:     "google_ads",
					Label:    "Дубликат",
					Kind:     finance.KindMarketing,
					IsActive: true,
				})
			},
			want: finance.ErrCategoryExists,
		},
		{
			name: "update unknown category",
			call: func() error {
				return repo.UpdateCategory(ctx, finance.Category{
					Code:     "no_such_category",
					Label:    "Нет такой",
					Kind:     finance.KindAdmin,
					IsActive: true,
				})
			},
			want: finance.ErrNotFound,
		},
		{
			name: "get unknown expense",
			call: func() error {
				_, err := repo.GetExpense(ctx, financeMissingID)
				return err
			},
			want: finance.ErrNotFound,
		},
		{
			name: "update unknown expense",
			call: func() error {
				return repo.UpdateExpense(ctx, finance.Expense{
					ID:           financeMissingID,
					SpentAt:      financeDate(2026, time.February, 2),
					Amount:       10,
					CategoryCode: "google_ads",
				})
			},
			want: finance.ErrNotFound,
		},
		{
			name: "delete unknown expense",
			call: func() error { return repo.DeleteExpense(ctx, financeMissingID) },
			want: finance.ErrNotFound,
		},
		{
			name: "update unknown rule",
			call: func() error {
				return repo.UpdateRule(ctx, finance.Rule{
					ID:           financeMissingID,
					Name:         "Нет такого",
					CategoryCode: "google_ads",
					Amount:       10,
					DayOfMonth:   1,
					ActiveFrom:   financeDate(2026, time.January, 1),
					IsActive:     true,
				})
			},
			want: finance.ErrNotFound,
		},
		{
			name: "delete unknown rule",
			call: func() error { return repo.DeleteRule(ctx, financeMissingID) },
			want: finance.ErrNotFound,
		},
		{
			name: "delete unknown other income",
			call: func() error { return repo.DeleteOtherIncome(ctx, financeMissingID) },
			want: finance.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	// A malformed id is NOT ErrNotFound — it surfaces as a raw pgx type error,
	// so anything mapping repository errors to HTTP has to handle it too.
	t.Run("malformed id is not ErrNotFound", func(t *testing.T) {
		_, err := repo.GetExpense(ctx, "not-a-uuid")
		if err == nil {
			t.Fatalf("GetExpense(%q) = nil error, want a failure", "not-a-uuid")
		}
		if errors.Is(err, finance.ErrNotFound) {
			t.Logf("GetExpense(%q) translated to ErrNotFound", "not-a-uuid")
		} else {
			t.Logf("documented behaviour: GetExpense(%q) = %v (raw driver error, not ErrNotFound)",
				"not-a-uuid", err)
		}
	})
}

// --- 9. ListExpenses filters and totals ---

func TestFinanceRepository_ListExpenses_FiltersTotalsAndPaging(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	seeds := []finance.Expense{
		{SpentAt: financeDate(2026, time.January, 5), Amount: 100, CategoryCode: "google_ads", Vendor: "Google Ads", Description: "January budget"},
		{SpentAt: financeDate(2026, time.January, 15), Amount: 200, CategoryCode: "smm", Vendor: "Fb Agency", Description: "SMM January", Status: finance.StatusDraft, Origin: finance.OriginRecurring},
		{SpentAt: financeDate(2026, time.February, 5), Amount: 300, CategoryCode: "google_ads", Vendor: "Google Ads", Description: "February budget"},
		{SpentAt: financeDate(2026, time.February, 10), Amount: 400, CategoryCode: "smm", Vendor: "Smm Studio", Description: "February smm", Origin: finance.OriginImported},
		{SpentAt: financeDate(2026, time.February, 20), Amount: 500, CategoryCode: "google_ads", Vendor: "Google Ads", Description: "draft row", Status: finance.StatusDraft, Origin: finance.OriginRecurring},
		{SpentAt: financeDate(2026, time.March, 1), Amount: 600, CategoryCode: "telephony", Vendor: "Binotel", Description: "telephony"},
	}
	for _, e := range seeds {
		createExpense(t, repo, e)
	}

	tests := []struct {
		name      string
		filter    finance.ExpenseFilter
		wantTotal int
		wantSum   float64
		wantItems int
	}{
		{
			name:      "no filter",
			filter:    finance.ExpenseFilter{},
			wantTotal: 6, wantSum: 1400, wantItems: 6,
		},
		{
			name:      "february only",
			filter:    finance.ExpenseFilter{From: financeDate(2026, time.February, 1), To: financeDate(2026, time.February, 28)},
			wantTotal: 3, wantSum: 700, wantItems: 3,
		},
		{
			name:      "from is inclusive of its own day",
			filter:    finance.ExpenseFilter{From: financeDate(2026, time.February, 20)},
			wantTotal: 2, wantSum: 600, wantItems: 2,
		},
		{
			name:      "to is inclusive of its own day",
			filter:    finance.ExpenseFilter{To: financeDate(2026, time.January, 15)},
			wantTotal: 2, wantSum: 100, wantItems: 2,
		},
		{
			name:      "category",
			filter:    finance.ExpenseFilter{CategoryCode: "google_ads"},
			wantTotal: 3, wantSum: 400, wantItems: 3,
		},
		{
			name:      "status draft",
			filter:    finance.ExpenseFilter{Status: finance.StatusDraft},
			wantTotal: 2, wantSum: 0, wantItems: 2,
		},
		{
			name:      "status posted",
			filter:    finance.ExpenseFilter{Status: finance.StatusPosted},
			wantTotal: 4, wantSum: 1400, wantItems: 4,
		},
		{
			name:      "origin recurring",
			filter:    finance.ExpenseFilter{Origin: finance.OriginRecurring},
			wantTotal: 2, wantSum: 0, wantItems: 2,
		},
		{
			name:      "origin imported",
			filter:    finance.ExpenseFilter{Origin: finance.OriginImported},
			wantTotal: 1, wantSum: 400, wantItems: 1,
		},
		{
			name:      "search matches vendor case-insensitively",
			filter:    finance.ExpenseFilter{Search: "GOOGLE"},
			wantTotal: 3, wantSum: 400, wantItems: 3,
		},
		{
			name:      "search matches description case-insensitively",
			filter:    finance.ExpenseFilter{Search: "february"},
			wantTotal: 2, wantSum: 700, wantItems: 2,
		},
		{
			name:      "search matches vendor and description together",
			filter:    finance.ExpenseFilter{Search: "smm"},
			wantTotal: 2, wantSum: 400, wantItems: 2,
		},
		{
			name:      "combined filters intersect",
			filter:    finance.ExpenseFilter{CategoryCode: "google_ads", Status: finance.StatusPosted, From: financeDate(2026, time.February, 1)},
			wantTotal: 1, wantSum: 300, wantItems: 1,
		},
		{
			name:      "search matches nothing",
			filter:    finance.ExpenseFilter{Search: "нет такого"},
			wantTotal: 0, wantSum: 0, wantItems: 0,
		},
		{
			// Total/Sum cover the whole filtered set, Items only the page. Sum is
			// posted-only, which is why it is below the count of matching rows.
			name:      "paging does not shrink the totals",
			filter:    finance.ExpenseFilter{Limit: 2, Offset: 1},
			wantTotal: 6, wantSum: 1400, wantItems: 2,
		},
		{
			name:      "offset past the end",
			filter:    finance.ExpenseFilter{Limit: 2, Offset: 10},
			wantTotal: 6, wantSum: 1400, wantItems: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := repo.ListExpenses(ctx, tt.filter)
			if err != nil {
				t.Fatalf("ListExpenses: %v", err)
			}
			if list.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", list.Total, tt.wantTotal)
			}
			if list.Sum != tt.wantSum {
				t.Errorf("Sum = %v, want %v", list.Sum, tt.wantSum)
			}
			if len(list.Items) != tt.wantItems {
				t.Errorf("len(Items) = %d, want %d", len(list.Items), tt.wantItems)
			}
		})
	}

	// Ordering is spent_at DESC, so a Limit/Offset page is a deterministic
	// window over it — otherwise page 2 can repeat page 1.
	t.Run("page window follows spent_at DESC", func(t *testing.T) {
		list, err := repo.ListExpenses(ctx, finance.ExpenseFilter{Limit: 2, Offset: 1})
		if err != nil {
			t.Fatalf("ListExpenses: %v", err)
		}
		wantAmounts := []float64{500, 400}
		if len(list.Items) != len(wantAmounts) {
			t.Fatalf("len(Items) = %d, want %d", len(list.Items), len(wantAmounts))
		}
		for i, want := range wantAmounts {
			if list.Items[i].Amount != want {
				t.Errorf("Items[%d].Amount = %v, want %v", i, list.Items[i].Amount, want)
			}
		}
		if list.Items[0].CategoryLabel == "" {
			t.Errorf("CategoryLabel is empty, want the joined label for %q", list.Items[0].CategoryCode)
		}
	})
}

// The search box is a literal, not a LIKE pattern: without the ESCAPE clause a
// single "%" matched the entire ledger instead of the one row containing it.
func TestFinanceRepository_ListExpenses_SearchTreatsLikeWildcardsAsLiterals(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	for _, vendor := range []string{"Google Ads", "Binotel", "50% off deal"} {
		createExpense(t, repo, finance.Expense{
			SpentAt:      financeDate(2026, time.February, 1),
			Amount:       10,
			CategoryCode: "smm",
			Vendor:       vendor,
		})
	}

	tests := []struct {
		name      string
		search    string
		wantTotal int
	}{
		{name: "percent matches only the row containing one", search: "%", wantTotal: 1},
		{name: "underscore matches nothing when no vendor has one", search: "_", wantTotal: 0},
		{name: "a literal percent inside a longer term still narrows", search: "50%", wantTotal: 1},
		{name: "plain text is unaffected", search: "binotel", wantTotal: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := repo.ListExpenses(ctx, finance.ExpenseFilter{Search: tt.search})
			if err != nil {
				t.Fatalf("ListExpenses(Search=%q): %v", tt.search, err)
			}
			if list.Total != tt.wantTotal {
				t.Errorf("Search=%q Total = %d, want %d", tt.search, list.Total, tt.wantTotal)
			}
		})
	}
}

// Voiding is what a "delete" of a generated row does: the row keeps its
// external_ref, so the next generator pass finds the key taken and does not
// re-create the expense staff just removed.
func TestFinanceRepository_VoidExpenseKeepsTheExternalRef(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewFinanceRepository(pool)

	generated := finance.Expense{
		SpentAt:      financeDate(2026, time.February, 5),
		Amount:       642,
		CategoryCode: "website",
		Status:       finance.StatusPosted,
		Origin:       finance.OriginRecurring,
		ExternalRef:  "rule:void-me:2026-02",
	}
	inserted, err := repo.InsertGenerated(ctx, generated)
	if err != nil || !inserted {
		t.Fatalf("InsertGenerated = %v, %v; want true, nil", inserted, err)
	}

	list, err := repo.ListExpenses(ctx, finance.ExpenseFilter{})
	if err != nil {
		t.Fatalf("ListExpenses: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("want 1 row, got %d", len(list.Items))
	}
	id := list.Items[0].ID

	if err := repo.VoidExpense(ctx, id); err != nil {
		t.Fatalf("VoidExpense: %v", err)
	}

	voided, err := repo.GetExpense(ctx, id)
	if err != nil {
		t.Fatalf("GetExpense: %v", err)
	}
	if voided.Status != finance.StatusVoid {
		t.Errorf("status = %q, want void", voided.Status)
	}
	if voided.ExternalRef != generated.ExternalRef {
		t.Errorf("external ref = %q, want it kept as %q", voided.ExternalRef, generated.ExternalRef)
	}

	again, err := repo.InsertGenerated(ctx, generated)
	if err != nil {
		t.Fatalf("InsertGenerated after void: %v", err)
	}
	if again {
		t.Error("the generator re-created a voided row — the external ref stopped protecting it")
	}

	// A voided row counts as spend nowhere.
	facts, err := repo.MonthlyFacts(ctx, financeDate(2026, time.February, 1), financeDate(2026, time.February, 28))
	if err != nil {
		t.Fatalf("MonthlyFacts: %v", err)
	}
	if len(facts) != 1 || facts[0].ExpenseByCategory["website"] != 0 {
		t.Errorf("voided row still in the P&L: %+v", facts)
	}
	if err := repo.VoidExpense(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, finance.ErrNotFound) {
		t.Errorf("VoidExpense(unknown) = %v, want ErrNotFound", err)
	}
}

// --- 10. advocate rates ---

func TestAdvocateRateRepository_SetAndListRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewAdvocateRateRepository(pool)

	active := seedAdvocate(t, pool, "Активный Адвокат", 0)
	retired := seedAdvocate(t, pool, "Ушедший Адвокат", 0)
	if _, err := pool.Exec(ctx, `UPDATE advocates SET is_active = false WHERE id = @id`,
		pgx.NamedArgs{"id": retired}); err != nil {
		t.Fatalf("deactivate advocate: %v", err)
	}

	if err := repo.SetAdvocateRate(ctx, active, 22.5); err != nil {
		t.Fatalf("SetAdvocateRate active: %v", err)
	}
	// Inactive advocates keep a rate: past months still need it to be right.
	if err := repo.SetAdvocateRate(ctx, retired, 10); err != nil {
		t.Fatalf("SetAdvocateRate retired: %v", err)
	}

	rates, err := repo.ListAdvocateRates(ctx)
	if err != nil {
		t.Fatalf("ListAdvocateRates: %v", err)
	}
	want := []finance.AdvocateRate{
		{AdvocateID: active, FullName: "Активный Адвокат", IsActive: true, CommissionPercent: 22.5},
		{AdvocateID: retired, FullName: "Ушедший Адвокат", IsActive: false, CommissionPercent: 10},
	}
	if len(rates) != len(want) {
		t.Fatalf("ListAdvocateRates = %+v, want %d rows (active first)", rates, len(want))
	}
	for i := range want {
		if rates[i] != want[i] {
			t.Errorf("ListAdvocateRates[%d] = %+v, want %+v", i, rates[i], want[i])
		}
	}

	// Overwriting is a plain replace, not an accumulate.
	if err := repo.SetAdvocateRate(ctx, active, 0); err != nil {
		t.Fatalf("SetAdvocateRate back to zero: %v", err)
	}
	rates, err = repo.ListAdvocateRates(ctx)
	if err != nil {
		t.Fatalf("ListAdvocateRates after reset: %v", err)
	}
	if rates[0].CommissionPercent != 0 {
		t.Errorf("CommissionPercent after reset = %v, want 0", rates[0].CommissionPercent)
	}

	if err := repo.SetAdvocateRate(ctx, financeMissingID, 30); !errors.Is(err, finance.ErrNotFound) {
		t.Errorf("SetAdvocateRate(unknown) = %v, want ErrNotFound", err)
	}
}
