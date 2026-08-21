package finance

import (
	"fmt"
	"math"
	"time"
)

// MonthKey is the canonical month identity used in external refs and report rows.
func MonthKey(t time.Time) string {
	return t.Format("2006-01")
}

// Every generated row carries one of these in expenses.external_ref, and the
// unique index on that column is what makes re-running the generator a no-op
// instead of a double charge.
func RecurringRef(ruleID string, month time.Time) string {
	return fmt.Sprintf("rule:%s:%s", ruleID, MonthKey(month))
}

func AdvocatePayoutRef(advocateID string, month time.Time) string {
	return fmt.Sprintf("advocate:%s:%s", advocateID, MonthKey(month))
}

func ImportRef(tab string, row int) string {
	return fmt.Sprintf("sheet:%s:%d", tab, row)
}

// DueDate is when this rule falls in the given month; DayOfMonth is capped at
// 28 by the schema, so no month is ever skipped and no date ever rolls over.
func (r Rule) DueDate(month time.Time) time.Time {
	return time.Date(month.Year(), month.Month(), r.DayOfMonth, 0, 0, 0, 0, month.Location())
}

// Planned is a rule occurrence the generator should write; Amount is the
// rule's expected amount, which staff can still correct when confirming.
type Planned struct {
	Rule        Rule
	SpentAt     time.Time
	Amount      float64
	ExternalRef string
	Status      Status
}

// PlanRecurring returns the occurrences due in month as of now. Days later
// than now are deliberately left out: an expense that hasn't happened yet
// must not sit in the ledger, so a mid-month run generates only what already
// came due and a later run picks up the rest.
func PlanRecurring(rules []Rule, month time.Time, now time.Time) []Planned {
	out := make([]Planned, 0, len(rules))
	for _, r := range rules {
		if !r.IsActive {
			continue
		}
		due := r.DueDate(month)
		if due.Before(dayOf(r.ActiveFrom)) {
			continue
		}
		if r.ActiveTo != nil && due.After(dayOf(*r.ActiveTo)) {
			continue
		}
		if due.After(dayOf(now)) {
			continue
		}
		status := StatusDraft
		if r.AutoPost {
			status = StatusPosted
		}
		out = append(out, Planned{
			Rule:        r,
			SpentAt:     due,
			Amount:      r.Amount,
			ExternalRef: RecurringRef(r.ID, month),
			Status:      status,
		})
	}
	return out
}

// NewAdvocateSettlement turns a period's collections and what was already paid
// into the four numbers staff asks for: collected, accrued, paid, still owed.
func NewAdvocateSettlement(c AdvocateCollection, paid float64) AdvocateSettlement {
	accrued := roundMoney(c.Collected * c.CommissionPercent / 100)
	return AdvocateSettlement{
		AdvocateID:        c.AdvocateID,
		FullName:          c.AdvocateName,
		CommissionPercent: c.CommissionPercent,
		Collected:         roundMoney(c.Collected),
		Accrued:           accrued,
		Paid:              roundMoney(paid),
		Outstanding:       roundMoney(accrued - paid),
	}
}

// AdvocateCollection is what one advocate actually collected in a month, per
// case_payments — the base their payout is a percentage of.
type AdvocateCollection struct {
	AdvocateID        string
	AdvocateName      string
	Collected         float64
	CommissionPercent float64
}

// Payout is always a draft, never auto-posted: it is a payment to a person,
// and the historical lump sums in the spreadsheet show the percentage is a
// starting point staff adjusts, not a final number.
type Payout struct {
	AdvocateID   string
	AdvocateName string
	Collected    float64
	Percent      float64
	Amount       float64
	SpentAt      time.Time
	ExternalRef  string
}

// PlanAdvocatePayouts only plans for a month that has already ended. The payout
// amount is a snapshot of what was collected, but its idempotency key is
// month-scoped — so planning mid-month would freeze whatever partial figure the
// first run happened to see (5 000 collected on the 2nd) and never update it as
// the month fills up (45 000 by the 31st). Advocates with a zero percentage (the
// roster default — nothing is invented for them) or zero collections are skipped.
func PlanAdvocatePayouts(collections []AdvocateCollection, month time.Time, now time.Time) []Payout {
	spentAt := endOfMonth(month)
	if !spentAt.Before(dayOf(now)) {
		return nil
	}
	out := make([]Payout, 0, len(collections))
	for _, c := range collections {
		if c.CommissionPercent <= 0 || c.Collected <= 0 {
			continue
		}
		amount := roundMoney(c.Collected * c.CommissionPercent / 100)
		if amount <= 0 {
			continue
		}
		out = append(out, Payout{
			AdvocateID:   c.AdvocateID,
			AdvocateName: c.AdvocateName,
			Collected:    c.Collected,
			Percent:      c.CommissionPercent,
			Amount:       amount,
			SpentAt:      spentAt,
			ExternalRef:  AdvocatePayoutRef(c.AdvocateID, month),
		})
	}
	return out
}

func endOfMonth(month time.Time) time.Time {
	first := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
	return first.AddDate(0, 1, -1)
}

func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}
