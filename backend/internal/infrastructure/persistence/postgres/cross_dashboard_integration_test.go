//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/finance"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

// The finance P&L and the leads dashboard read collected money from different
// places: /finance sums the case_payments ledger, /leads sums cases.paid_amount.
// They agreed right up until historical cases turned out to carry the total in
// the column with no ledger row behind it — the same 100 000 ₴ then read as
// 100 000 on one page and 0 on the other. This pins the invariant that matters
// to whoever reads those pages: for the same period, the two must report the
// same money.
func TestFinanceAndLeadStatsAgreeOnCollectedMoney(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)

	from := financeDate(2026, time.March, 1)
	to := financeDate(2026, time.March, 31)
	inMarch := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)

	clientID := seedClient(t, pool, "+380990000100", "Сверка")
	// Booked and held in the same month on purpose: the two pages bucket
	// consultation revenue by different columns (created_at vs scheduled_at),
	// and this test is about WHERE the money is read from, not about which
	// month it lands in.
	seedConsultationDated(t, pool, clientID, inMarch, inMarch, 800, "completed")
	caseID := seedCaseWithMoney(t, pool, clientID, "Борзов", 15000, 15000, inMarch)
	seedCasePayment(t, pool, caseID, 15000, inMarch)

	facts, err := postgres.NewFinanceRepository(pool).MonthlyFacts(ctx, from, to)
	if err != nil {
		t.Fatalf("MonthlyFacts: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("want one month, got %d", len(facts))
	}
	financeIncome := facts[0].ConsultRevenue + facts[0].CasePaid

	totals, err := postgres.NewLeadStatsRepository(pool).Totals(ctx, from, to)
	if err != nil {
		t.Fatalf("Totals: %v", err)
	}
	leadsIncome := totals.RevenueEarned + totals.CasePaid

	if financeIncome != leadsIncome {
		t.Fatalf("/finance reports %.2f collected, /leads reports %.2f — the same money must not read differently on two pages",
			financeIncome, leadsIncome)
	}
	if financeIncome != 15800 {
		t.Errorf("collected = %.2f, want 15800 (800 consultation + 15000 case)", financeIncome)
	}
}

// Money in cases.paid_amount with nothing in case_payments is the shape that
// broke the P&L: leadstats sees it, finance does not. Nothing writes it that way
// any more (cmd/importcases inserts the ledger row too, and AddPayment always
// did), so the query below is the check to run against a real database after any
// bulk import.
func TestNoCaseHoldsMoneyOutsideThePaymentLedger(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)

	clientID := seedClient(t, pool, "+380990000101", "Только колонка")
	caseID := seedCaseWithMoney(t, pool, clientID, "Борзов", 9000, 9000, time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC))

	const q = `
		SELECT count(*) FROM cases c
		WHERE c.paid_amount > 0
			AND NOT EXISTS (SELECT 1 FROM case_payments p WHERE p.case_id = c.id)`

	var orphans int
	if err := pool.QueryRow(ctx, q).Scan(&orphans); err != nil {
		t.Fatalf("orphan query: %v", err)
	}
	if orphans != 1 {
		t.Fatalf("the check itself is broken: seeded one column-only case, found %d", orphans)
	}

	seedCasePayment(t, pool, caseID, 9000, time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC))
	if err := pool.QueryRow(ctx, q).Scan(&orphans); err != nil {
		t.Fatalf("orphan query after backfill: %v", err)
	}
	if orphans != 0 {
		t.Errorf("after the ledger row exists the case must not count as an orphan, got %d", orphans)
	}
}

func seedConsultationDated(t *testing.T, pool *pgxpool.Pool, clientID string, createdAt, scheduledAt time.Time, price float64, status string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO consultations (client_id, scheduled_at, price, status, created_at)
			VALUES (@client_id, @scheduled_at, @price, @status, @created_at)`,
		pgx.NamedArgs{
			"client_id":    clientID,
			"scheduled_at": scheduledAt,
			"price":        price,
			"status":       status,
			"created_at":   createdAt,
		})
	if err != nil {
		t.Fatalf("seed consultation: %v", err)
	}
}

func seedCaseWithMoney(t *testing.T, pool *pgxpool.Pool, clientID, advocateName string, fee, paid float64, createdAt time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO cases (client_id, advocate_name, fee, paid_amount, created_at)
			VALUES (@client_id, @advocate_name, @fee, @paid, @created_at) RETURNING id`,
		pgx.NamedArgs{
			"client_id":     clientID,
			"advocate_name": advocateName,
			"fee":           fee,
			"paid":          paid,
			"created_at":    createdAt,
		}).Scan(&id)
	if err != nil {
		t.Fatalf("seed case: %v", err)
	}
	return id
}

// A consultation that happened but was not paid must vanish from BOTH pages'
// revenue at once. Before consultations carried a payment answer, "провів" alone
// booked the money and there was no way to say it never arrived.
func TestUnpaidConsultationCountsOnNeitherPage(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)

	from := financeDate(2026, time.March, 1)
	to := financeDate(2026, time.March, 31)
	inMarch := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)

	clientID := seedClient(t, pool, "+380990000200", "Не заплатил")
	seedConsultationDated(t, pool, clientID, inMarch, inMarch, 800, "completed")
	seedConsultationDated(t, pool, clientID, inMarch.AddDate(0, 0, 1), inMarch.AddDate(0, 0, 1), 2000, "completed")

	financeRepo := postgres.NewFinanceRepository(pool)
	leadStatsRepo := postgres.NewLeadStatsRepository(pool)

	revenue := func() (float64, float64) {
		t.Helper()
		facts, err := financeRepo.MonthlyFacts(ctx, from, to)
		if err != nil {
			t.Fatalf("MonthlyFacts: %v", err)
		}
		totals, err := leadStatsRepo.Totals(ctx, from, to)
		if err != nil {
			t.Fatalf("Totals: %v", err)
		}
		return facts[0].ConsultRevenue, totals.RevenueEarned
	}

	// Nobody has been asked yet: both pages count both consultations, exactly as
	// they did before the column existed.
	if fin, leads := revenue(); fin != 2800 || leads != 2800 {
		t.Fatalf("unanswered: finance %v, leads %v, want 2800 each", fin, leads)
	}

	if _, err := pool.Exec(ctx, `UPDATE consultations SET paid = false WHERE price = 800`); err != nil {
		t.Fatalf("mark unpaid: %v", err)
	}

	fin, leads := revenue()
	if fin != 2000 {
		t.Errorf("finance revenue = %v, want 2000 — the unpaid 800 is a debt, not income", fin)
	}
	if leads != 2000 {
		t.Errorf("leads revenue = %v, want 2000 — the same money must not read differently on two pages", leads)
	}

	gaps, err := financeRepo.DataGaps(ctx, from, to)
	if err != nil {
		t.Fatalf("DataGaps: %v", err)
	}
	var found bool
	for _, g := range gaps {
		if g.Kind == finance.GapUnpaidCompleted {
			found = true
			if g.Count != 1 || g.Amount != 800 {
				t.Errorf("unpaid gap = %+v, want 1 row of 800", g)
			}
		}
	}
	if !found {
		t.Error("the unpaid consultation must surface as a gap, or the money simply disappears")
	}
}
