package finance

import (
	"context"
	"time"
)

type CategoryStore interface {
	ListCategories(ctx context.Context, activeOnly bool) ([]Category, error)
	// CreateCategory returns ErrCategoryExists when code is taken.
	CreateCategory(ctx context.Context, c Category) error
	// UpdateCategory replaces label/kind/is_active/sort_order; returns ErrNotFound.
	UpdateCategory(ctx context.Context, c Category) error
}

type ExpenseStore interface {
	ListExpenses(ctx context.Context, filter ExpenseFilter) (ExpenseList, error)
	GetExpense(ctx context.Context, id string) (Expense, error)
	CreateExpense(ctx context.Context, e Expense) (Expense, error)
	UpdateExpense(ctx context.Context, e Expense) error
	DeleteExpense(ctx context.Context, id string) error
	// ConfirmExpense flips draft to posted; returns ErrNotDraft for a posted row.
	ConfirmExpense(ctx context.Context, id string) error
	// VoidExpense retires a generated row without freeing its ExternalRef.
	VoidExpense(ctx context.Context, id string) error
	// InsertGenerated is the idempotent write behind the generator: false, nil
	// means the ExternalRef was already there and nothing was inserted.
	InsertGenerated(ctx context.Context, e Expense) (bool, error)
}

type RuleStore interface {
	ListRules(ctx context.Context, activeOnly bool) ([]Rule, error)
	CreateRule(ctx context.Context, r Rule) (Rule, error)
	UpdateRule(ctx context.Context, r Rule) error
	DeleteRule(ctx context.Context, id string) error
}

type IncomeStore interface {
	ListOtherIncome(ctx context.Context, from, to time.Time) ([]OtherIncome, error)
	CreateOtherIncome(ctx context.Context, i OtherIncome) (OtherIncome, error)
	DeleteOtherIncome(ctx context.Context, id string) error
}

type AdvocateRateStore interface {
	ListAdvocateRates(ctx context.Context) ([]AdvocateRate, error)
	// SetAdvocateRate returns ErrNotFound for an unknown advocate.
	SetAdvocateRate(ctx context.Context, advocateID string, percent float64) error
}

// ReportSource is the read side over CRM data the P&L needs but doesn't own.
type ReportSource interface {
	// MonthlyFacts returns one row per month in [from, to] ascending, including
	// months with no activity — a gap in the middle must not shift the columns.
	MonthlyFacts(ctx context.Context, from, to time.Time) ([]MonthFacts, error)
	// BalanceBefore is the cumulative balance of everything before from, so a
	// report window that starts mid-history still shows a true running total.
	BalanceBefore(ctx context.Context, from time.Time) (float64, error)
	// Receivable is what is still owed across every case — live, not scoped to
	// any window.
	Receivable(ctx context.Context) (float64, error)
	// DataRange returns the span of rows that carry money, plus the latest date
	// of any activity at all (leads included). Zero times mean nothing yet.
	DataRange(ctx context.Context) (firstMoney, lastMoney, lastActivity time.Time, err error)
	// AdvocateCollections is per-advocate case_payments received in [from, to],
	// paired with that advocate's commission percent.
	AdvocateCollections(ctx context.Context, from, to time.Time) ([]AdvocateCollection, error)
	// AdvocatePayouts is what was actually paid out of the advocates category in
	// [from, to]: attributed per advocate id where the row's external ref names
	// one, plus everything that names nobody.
	AdvocatePayouts(ctx context.Context, from, to time.Time) (attributed map[string]float64, unattributed float64, err error)
}

// AdSpend is one month of advertising cost as the ad platform itself reports
// it, which is not the same thing as what the company paid that month: the
// ledger records card top-ups of the ad account, the platform reports what it
// actually consumed. Both are real; the P&L is cash-basis, so this exists to
// be compared against the ledger, not to silently replace it.
type AdSpend struct {
	Month  string // MonthKey format
	Cost   float64
	Clicks int64
}
