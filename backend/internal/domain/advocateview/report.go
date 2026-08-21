package advocateview

import (
	"math"
	"sort"
	"time"
)

// NewSettlement accrues the advocate's percentage month by month rather than
// on the grand total: the percentage can change, and a month already accrued
// at 35% must not silently re-price when the rate moves. (Today the roster
// keeps a single current percentage, so both give the same number — the
// per-month shape is what makes a rate history a data change instead of a
// rewrite.)
func NewSettlement(a Advocate, months []MonthMoney, paid float64) Settlement {
	out := Settlement{
		AdvocateID:        a.ID,
		FullName:          a.FullName,
		CommissionPercent: a.CommissionPercent,
		Paid:              roundMoney(paid),
		PaidIsPartial:     true,
		Months:            make([]MonthMoney, 0, len(months)),
	}
	for _, m := range AccrueMonths(months, a.CommissionPercent) {
		out.Collected += m.Collected
		out.Accrued += m.Accrued
		out.Months = append(out.Months, m)
	}
	out.Collected = roundMoney(out.Collected)
	out.Accrued = roundMoney(out.Accrued)
	out.Outstanding = roundMoney(out.Accrued - out.Paid)
	return out
}

// AccrueMonths prices each month of collections at the advocate's percentage.
// Both the settlement and the stats show this series, and they must not each
// have their own idea of how the rounding goes.
func AccrueMonths(months []MonthMoney, percent float64) []MonthMoney {
	out := make([]MonthMoney, 0, len(months))
	for _, m := range months {
		m.Collected = roundMoney(m.Collected)
		m.Accrued = roundMoney(m.Collected * percent / 100)
		out = append(out, m)
	}
	return out
}

// NewStats counts what the advocate's own case list already contains. Money
// per month comes from the payment ledger (months), not from the cases' running
// totals, so "collected in May" means payments dated May and not the fee of a
// case opened in May.
func NewStats(cases []Case, months []MonthMoney) Stats {
	out := Stats{Cases: len(cases), Months: months}

	perStatus := map[string]int{}
	clients := map[string]struct{}{}
	for _, c := range cases {
		perStatus[c.Status]++
		clients[c.ClientID] = struct{}{}
		out.FeeTotal += c.Fee
		out.PaidTotal += c.Paid
		out.ClientDebt += c.Owed()
		if out.FirstCaseAt.IsZero() || c.CreatedAt.Before(out.FirstCaseAt) {
			out.FirstCaseAt = c.CreatedAt
		}
		for _, p := range c.Payments {
			if p.PaidAt.After(out.LastPaymentAt) {
				out.LastPaymentAt = p.PaidAt
			}
		}
	}

	out.Clients = len(clients)
	out.FeeTotal = roundMoney(out.FeeTotal)
	out.PaidTotal = roundMoney(out.PaidTotal)
	out.ClientDebt = roundMoney(out.ClientDebt)
	if out.Cases > 0 {
		out.AvgFee = roundMoney(out.FeeTotal / float64(out.Cases))
	}

	out.ByStatus = make([]StatusCount, 0, len(perStatus))
	for status, count := range perStatus {
		out.ByStatus = append(out.ByStatus, StatusCount{Status: status, Count: count})
	}
	// Map iteration order is random; a scoreboard that reshuffles between two
	// refreshes of the same page reads as broken.
	sort.Slice(out.ByStatus, func(i, j int) bool {
		if out.ByStatus[i].Count != out.ByStatus[j].Count {
			return out.ByStatus[i].Count > out.ByStatus[j].Count
		}
		return out.ByStatus[i].Status < out.ByStatus[j].Status
	})
	return out
}

// MonthKey is the month a date belongs to in the same YYYY-MM form the P&L
// uses, so both pages label the same month the same way.
func MonthKey(t time.Time) string {
	return t.Format("2006-01")
}

func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}
