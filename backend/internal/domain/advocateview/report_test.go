package advocateview

import (
	"testing"
	"time"
)

func TestNewSettlementAccruesPerMonth(t *testing.T) {
	advocate := Advocate{ID: "borzov", FullName: "Ярослав Борзов", CommissionPercent: 35}
	months := []MonthMoney{
		{Month: "2026-05", Collected: 12000},
		{Month: "2026-06", Collected: 3000},
	}

	got := NewSettlement(advocate, months, 4200)

	if got.Collected != 15000 {
		t.Errorf("collected = %v, want 15000", got.Collected)
	}
	if got.Accrued != 5250 {
		t.Errorf("accrued = %v, want 5250", got.Accrued)
	}
	if got.Outstanding != 1050 {
		t.Errorf("outstanding = %v, want 1050", got.Outstanding)
	}
	if len(got.Months) != 2 || got.Months[0].Accrued != 4200 || got.Months[1].Accrued != 1050 {
		t.Errorf("months = %+v, want each month priced at 35%%", got.Months)
	}
	if !got.PaidIsPartial {
		t.Error("paid_is_partial must stay true — only generated payouts are attributable")
	}
}

// Rounding happens per month, not once at the end: the monthly figures on
// screen have to add up to the total next to them, or the page argues with
// itself.
func TestNewSettlementMonthsSumToTheTotal(t *testing.T) {
	advocate := Advocate{CommissionPercent: 33.333}
	months := []MonthMoney{
		{Month: "2026-01", Collected: 1000},
		{Month: "2026-02", Collected: 1000},
		{Month: "2026-03", Collected: 1000},
	}

	got := NewSettlement(advocate, months, 0)

	var sum float64
	for _, m := range got.Months {
		sum += m.Accrued
	}
	if sum != got.Accrued {
		t.Errorf("months sum to %v but the total says %v", sum, got.Accrued)
	}
}

func TestNewSettlementWithNoMoney(t *testing.T) {
	got := NewSettlement(Advocate{ID: "new", CommissionPercent: 35}, nil, 0)

	if got.Collected != 0 || got.Accrued != 0 || got.Outstanding != 0 {
		t.Errorf("an advocate with no payments yet = %+v, want zeros", got)
	}
	if got.Months == nil {
		t.Error("months must be an empty list, not nil — the JSON contract says array")
	}
}

// A payout larger than what the percentage accrues (an advance, or a lump sum
// that happens to carry the ref) shows as a negative remainder rather than
// being clamped: "we already paid you more than you earned this period" is a
// real state, and hiding it would make the number wrong.
func TestNewSettlementShowsOverpayment(t *testing.T) {
	got := NewSettlement(Advocate{CommissionPercent: 35}, []MonthMoney{{Month: "2026-05", Collected: 1000}}, 1000)

	if got.Outstanding != -650 {
		t.Errorf("outstanding = %v, want -650", got.Outstanding)
	}
}

func TestNewStats(t *testing.T) {
	may := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	june := time.Date(2026, time.June, 20, 0, 0, 0, 0, time.UTC)

	cases := []Case{
		{
			ID: "a", ClientID: "client-1", Status: "in_progress", Fee: 12000, Paid: 4000, CreatedAt: june,
			Payments: []Payment{{Amount: 4000, PaidAt: june}},
		},
		{
			ID: "b", ClientID: "client-1", Status: "completed", Fee: 8000, Paid: 8000, CreatedAt: may,
			Payments: []Payment{{Amount: 8000, PaidAt: may}},
		},
		{ID: "c", ClientID: "client-2", Status: "completed", Fee: 4000, Paid: 0, CreatedAt: may},
	}

	got := NewStats(cases, []MonthMoney{{Month: "2026-05", Collected: 8000, Accrued: 2800}})

	if got.Cases != 3 {
		t.Errorf("cases = %d, want 3", got.Cases)
	}
	if got.Clients != 2 {
		t.Errorf("clients = %d, want 2 — two cases share a client", got.Clients)
	}
	if got.FeeTotal != 24000 || got.PaidTotal != 12000 {
		t.Errorf("fee = %v, paid = %v, want 24000 / 12000", got.FeeTotal, got.PaidTotal)
	}
	if got.ClientDebt != 12000 {
		t.Errorf("client debt = %v, want 12000", got.ClientDebt)
	}
	if got.AvgFee != 8000 {
		t.Errorf("avg fee = %v, want 8000", got.AvgFee)
	}
	if !got.FirstCaseAt.Equal(may) {
		t.Errorf("first case = %v, want %v", got.FirstCaseAt, may)
	}
	if !got.LastPaymentAt.Equal(june) {
		t.Errorf("last payment = %v, want %v", got.LastPaymentAt, june)
	}
	if len(got.ByStatus) != 2 || got.ByStatus[0].Status != "completed" || got.ByStatus[0].Count != 2 {
		t.Errorf("by status = %+v, want completed first with 2", got.ByStatus)
	}
}

// A case paid beyond its fee (a client rounding up, or a fee lowered after the
// fact) must not report negative debt and quietly cancel out another case's.
func TestNewStatsDebtNeverGoesNegative(t *testing.T) {
	got := NewStats([]Case{
		{ID: "over", Status: "completed", Fee: 1000, Paid: 1500},
		{ID: "owing", Status: "in_progress", Fee: 5000, Paid: 1000},
	}, nil)

	if got.ClientDebt != 4000 {
		t.Errorf("client debt = %v, want 4000 — the overpaid case contributes 0, not -500", got.ClientDebt)
	}
}

func TestNewStatsEmpty(t *testing.T) {
	got := NewStats(nil, nil)

	if got.Cases != 0 || got.Clients != 0 || got.AvgFee != 0 {
		t.Errorf("stats for nothing = %+v, want zeros", got)
	}
	if got.ByStatus == nil {
		t.Error("by_status must be an empty list, not nil")
	}
}

func TestMonthKey(t *testing.T) {
	if got := MonthKey(time.Date(2026, time.January, 31, 23, 0, 0, 0, time.UTC)); got != "2026-01" {
		t.Errorf("month key = %q, want 2026-01", got)
	}
}
