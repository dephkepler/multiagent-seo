// Package finance is the company P&L: expenses (entered, generated, or
// imported) against income that stays owned by consultations/cases.
package finance

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("finance: not found")
var ErrCategoryExists = errors.New("finance: category already exists")
var ErrUnknownCategory = errors.New("finance: unknown category")
var ErrInvalidKind = errors.New("finance: invalid category kind")
var ErrInvalidAmount = errors.New("finance: amount must be positive")
var ErrInvalidName = errors.New("finance: name is empty")
var ErrInvalidPaymentMethod = errors.New("finance: invalid payment method")
var ErrInvalidDayOfMonth = errors.New("finance: day of month must be 1..28")
var ErrInvalidPercent = errors.New("finance: commission percent must be 0..100")

// Confirm applies to drafts only — a posted row is corrected via update, not re-confirmed.
var ErrNotDraft = errors.New("finance: expense is not a draft")

// Kind drives the derived numbers, not just grouping: KindMarketing is the
// CAC/ROMI denominator, KindDirect is what gross margin subtracts.
type Kind string

const (
	KindMarketing   Kind = "marketing"
	KindDirect      Kind = "direct"
	KindPayroll     Kind = "payroll"
	KindDevelopment Kind = "development"
	KindInfra       Kind = "infra"
	KindAdmin       Kind = "admin"
)

func IsKind(k Kind) bool {
	switch k {
	case KindMarketing, KindDirect, KindPayroll, KindDevelopment, KindInfra, KindAdmin:
		return true
	default:
		return false
	}
}

type PaymentMethod string

const (
	PaymentCard    PaymentMethod = "card"
	PaymentInvoice PaymentMethod = "invoice"
	// paid by the company account rather than the personal card — the spreadsheet's "от компании"
	PaymentCompany PaymentMethod = "company"
	PaymentCash    PaymentMethod = "cash"
)

func IsPaymentMethod(m PaymentMethod) bool {
	switch m {
	case PaymentCard, PaymentInvoice, PaymentCompany, PaymentCash:
		return true
	default:
		return false
	}
}

// A draft is generated, not yet acknowledged by staff; it stays out of the P&L until confirmed.
type Status string

const (
	StatusDraft  Status = "draft"
	StatusPosted Status = "posted"
	// a generated row staff removed: excluded from the P&L, but it keeps its
	// external_ref so the generator does not re-create it
	StatusVoid Status = "void"
)

type Origin string

const (
	OriginManual    Origin = "manual"
	OriginRecurring Origin = "recurring"
	// computed from CRM data — currently advocate payouts off case_payments
	OriginDerived  Origin = "derived"
	OriginImported Origin = "imported"
)

// CategoryAdvocates is seeded by the migration and is where derived advocate
// payouts are written — renaming its label is safe, the code is not.
const CategoryAdvocates = "advocates"

type Category struct {
	Code  string
	Label string
	Kind  Kind
	// IsPeoplePay marks money paid to a person for work, cutting across Kind:
	// copywriting is marketing spend AND a person's fee. Without it the P&L can
	// say what the money was for but not what the team costs.
	IsPeoplePay bool
	IsActive    bool
	SortOrder   int
}

type Expense struct {
	ID            string
	SpentAt       time.Time
	Amount        float64
	CategoryCode  string
	CategoryLabel string // joined for display; ignored on write
	PaymentMethod PaymentMethod
	Vendor        string
	Description   string
	Status        Status
	Origin        Origin
	RuleID        string
	ExternalRef   string
	CreatedBy     string
	CreatedAt     time.Time
}

// Rule is one recurring expense template; the generator turns it into an Expense once per month.
type Rule struct {
	ID            string
	Name          string
	CategoryCode  string
	CategoryLabel string
	Vendor        string
	PaymentMethod PaymentMethod
	Amount        float64
	DayOfMonth    int
	// true skips the draft step and writes straight to the ledger
	AutoPost   bool
	ActiveFrom time.Time
	ActiveTo   *time.Time // nil — open-ended
	IsActive   bool
	CreatedBy  string
	CreatedAt  time.Time
}

type OtherIncome struct {
	ID          string
	ReceivedAt  time.Time
	Amount      float64
	Source      string
	Description string
	// set only on rows written by the reconciliation top-up, so a repeat run
	// cannot book the same missing month twice
	ExternalRef string
	CreatedBy   string
	CreatedAt   time.Time
}

// AdvocateRate is an advocate's share of what they collect. 0 means no
// automatic payout is generated for them — the roster default, so nothing is
// ever guessed for an advocate whose real rate nobody entered.
type AdvocateRate struct {
	AdvocateID        string
	FullName          string
	IsActive          bool
	CommissionPercent float64
}

// AdvocateSettlement answers "how much do we still owe this advocate": what
// they collected, what their percentage accrues on it, what was actually paid
// out, and the remainder. Paid is only attributable for generated payouts,
// which carry the advocate in their external ref — the historical lump sums
// imported from the spreadsheet name no advocate at all, so they are reported
// separately rather than spread across people by guesswork.
type AdvocateSettlement struct {
	AdvocateID        string
	FullName          string
	IsActive          bool
	CommissionPercent float64
	Collected         float64
	Accrued           float64
	Paid              float64
	Outstanding       float64
}

// Settlement is the advocate side of the expenses, plus the income split it is
// computed against — "what consultations brought and what cases brought" is the
// same question one step up.
type Settlement struct {
	Advocates []AdvocateSettlement
	// payouts in the advocates category that name no advocate (imported history)
	UnattributedPaid float64
	ConsultIncome    float64
	CaseIncome       float64
	// sums across Advocates, so the UI has one number to show collapsed
	TotalAccrued     float64
	TotalPaid        float64
	TotalOutstanding float64
}

// Generated is what one generator pass did; Skipped counts occurrences that
// were already in the ledger, which is the normal case on a repeat run.
type Generated struct {
	Month     string
	Recurring int
	Payouts   int
	Skipped   int
}

// zero field = no filter, except Limit (defaults to defaultLimit in the service)
type ExpenseFilter struct {
	From          time.Time
	To            time.Time
	CategoryCode  string
	Status        Status
	Origin        Origin
	PaymentMethod PaymentMethod
	Search        string
	Limit         int
	Offset        int
}

type ExpenseList struct {
	Items []Expense
	Total int
	// Sum covers the whole filtered set, not just the current page.
	Sum float64
}

// DataGap is money the P&L cannot account for because of how a record was left,
// not because of arithmetic. Each one is a to-do for the person looking at the
// page: 59 200 ₴ sitting in consultations that were priced, booked and then
// never marked held or cancelled is not a rounding error, it is twelve
// conversations nobody closed.
//
// Kind is a stable string rather than a typed enum on purpose: the UI owns the
// wording and the advice, and adding a gap should not require touching a
// switch in three layers.
type DataGap struct {
	Kind   string
	Count  int
	Amount float64
}

const (
	// priced, booked, never resolved either way, and no case behind it
	GapUnresolvedConsultations = "unresolved_consultations"
	GapCancelledPriced         = "cancelled_priced"
	GapNoShowPriced            = "no_show_priced"
	GapZeroPricedCompleted     = "zero_priced_completed"
	GapFutureConsultations     = "future_consultations"
	GapUnlinkedCases           = "unlinked_cases"
	GapDuplicateAdvocates      = "duplicate_advocates"
)
