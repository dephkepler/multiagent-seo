package finance_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	appfinance "multiagent-seo/internal/application/finance"
	domain "multiagent-seo/internal/domain/finance"
)

var errBoom = errors.New("store is down")

// --- fakes ---

type fakeCategoryStore struct {
	categories      []domain.Category
	listErr         error
	activeOnlyCalls []bool
}

func (f *fakeCategoryStore) ListCategories(_ context.Context, activeOnly bool) ([]domain.Category, error) {
	f.activeOnlyCalls = append(f.activeOnlyCalls, activeOnly)
	if f.listErr != nil {
		return nil, f.listErr
	}
	if !activeOnly {
		return f.categories, nil
	}
	out := make([]domain.Category, 0, len(f.categories))
	for _, c := range f.categories {
		if c.IsActive {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeCategoryStore) CreateCategory(context.Context, domain.Category) error { return nil }

func (f *fakeCategoryStore) UpdateCategory(context.Context, domain.Category) error { return nil }

type fakeExpenseStore struct {
	rows        []domain.Expense
	refToRow    map[string]int
	nextID      int
	insertErr   error
	updateCalls []domain.Expense
}

func newFakeExpenseStore() *fakeExpenseStore {
	return &fakeExpenseStore{refToRow: map[string]int{}}
}

func (f *fakeExpenseStore) ListExpenses(_ context.Context, _ domain.ExpenseFilter) (domain.ExpenseList, error) {
	list := domain.ExpenseList{Items: f.rows, Total: len(f.rows)}
	for _, e := range f.rows {
		list.Sum += e.Amount
	}
	return list, nil
}

func (f *fakeExpenseStore) GetExpense(_ context.Context, id string) (domain.Expense, error) {
	for _, e := range f.rows {
		if e.ID == id {
			return e, nil
		}
	}
	return domain.Expense{}, domain.ErrNotFound
}

func (f *fakeExpenseStore) CreateExpense(_ context.Context, e domain.Expense) (domain.Expense, error) {
	f.nextID++
	e.ID = fmt.Sprintf("row-%d", f.nextID)
	e.CreatedAt = time.Now()
	f.rows = append(f.rows, e)
	return e, nil
}

// Mirrors the repository: status/origin/rule_id/external_ref are provenance,
// so an update rewrites only the editable columns. A service that started
// forcing a status would therefore be invisible here — the tests also assert
// on the value handed in (updateCalls).
func (f *fakeExpenseStore) UpdateExpense(_ context.Context, e domain.Expense) error {
	f.updateCalls = append(f.updateCalls, e)
	for i := range f.rows {
		if f.rows[i].ID != e.ID {
			continue
		}
		f.rows[i].SpentAt = e.SpentAt
		f.rows[i].Amount = e.Amount
		f.rows[i].CategoryCode = e.CategoryCode
		f.rows[i].PaymentMethod = e.PaymentMethod
		f.rows[i].Vendor = e.Vendor
		f.rows[i].Description = e.Description
		return nil
	}
	return domain.ErrNotFound
}

func (f *fakeExpenseStore) DeleteExpense(_ context.Context, id string) error {
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeExpenseStore) VoidExpense(_ context.Context, id string) error {
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows[i].Status = domain.StatusVoid
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeExpenseStore) ConfirmExpense(_ context.Context, id string) error {
	for i := range f.rows {
		if f.rows[i].ID != id {
			continue
		}
		if f.rows[i].Status != domain.StatusDraft {
			return domain.ErrNotDraft
		}
		f.rows[i].Status = domain.StatusPosted
		return nil
	}
	return domain.ErrNotFound
}

// InsertGenerated is the idempotent write: a ref already held inserts nothing
// and reports (false, nil), exactly like the on-conflict-do-nothing index.
func (f *fakeExpenseStore) InsertGenerated(_ context.Context, e domain.Expense) (bool, error) {
	if f.insertErr != nil {
		return false, f.insertErr
	}
	if e.ExternalRef != "" {
		if _, ok := f.refToRow[e.ExternalRef]; ok {
			return false, nil
		}
	}
	f.nextID++
	e.ID = fmt.Sprintf("gen-%d", f.nextID)
	e.CreatedAt = time.Now()
	f.rows = append(f.rows, e)
	if e.ExternalRef != "" {
		f.refToRow[e.ExternalRef] = len(f.rows) - 1
	}
	return true, nil
}

func (f *fakeExpenseStore) rowByRef(t *testing.T, ref string) domain.Expense {
	t.Helper()
	i, ok := f.refToRow[ref]
	if !ok {
		t.Fatalf("no row for external ref %q, have %d rows", ref, len(f.rows))
	}
	return f.rows[i]
}

func (f *fakeExpenseStore) countByRef(ref string) int {
	n := 0
	for _, e := range f.rows {
		if e.ExternalRef == ref {
			n++
		}
	}
	return n
}

type fakeRuleStore struct {
	rules   []domain.Rule
	listErr error
	created []domain.Rule
}

func (f *fakeRuleStore) ListRules(_ context.Context, activeOnly bool) ([]domain.Rule, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if !activeOnly {
		return f.rules, nil
	}
	out := make([]domain.Rule, 0, len(f.rules))
	for _, r := range f.rules {
		if r.IsActive {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRuleStore) CreateRule(_ context.Context, r domain.Rule) (domain.Rule, error) {
	r.ID = "rule-created"
	f.created = append(f.created, r)
	return r, nil
}

func (f *fakeRuleStore) UpdateRule(context.Context, domain.Rule) error { return nil }

func (f *fakeRuleStore) DeleteRule(context.Context, string) error { return nil }

type fakeIncomeStore struct {
	refs map[string]bool
}

func (f *fakeIncomeStore) ListOtherIncome(context.Context, time.Time, time.Time) ([]domain.OtherIncome, error) {
	return nil, nil
}

// mirrors the real store: an external ref already held inserts nothing
func (f *fakeIncomeStore) InsertOtherIncomeGenerated(_ context.Context, i domain.OtherIncome) (bool, error) {
	if f.refs == nil {
		f.refs = map[string]bool{}
	}
	if f.refs[i.ExternalRef] {
		return false, nil
	}
	f.refs[i.ExternalRef] = true
	return true, nil
}

func (f *fakeIncomeStore) CreateOtherIncome(_ context.Context, i domain.OtherIncome) (domain.OtherIncome, error) {
	i.ID = "income-created"
	return i, nil
}

func (f *fakeIncomeStore) DeleteOtherIncome(context.Context, string) error { return nil }

type advocateRateSet struct {
	advocateID string
	percent    float64
}

type fakeAdvocateRateStore struct {
	set []advocateRateSet
}

func (f *fakeAdvocateRateStore) ListAdvocateRates(context.Context) ([]domain.AdvocateRate, error) {
	return nil, nil
}

func (f *fakeAdvocateRateStore) SetAdvocateRate(_ context.Context, advocateID string, percent float64) error {
	f.set = append(f.set, advocateRateSet{advocateID, percent})
	return nil
}

type fakeReportSource struct {
	facts          []domain.MonthFacts
	factsErr       error
	opening        float64
	openingErr     error
	openingCalls   []time.Time
	collections    []domain.AdvocateCollection
	collectionsErr error
	receivable     float64
	receivableErr  error
	rangeFirst     time.Time
	rangeLast      time.Time
	rangeActivity  time.Time
	rangeErr       error

	payouts             map[string]float64
	payoutsUnattributed float64
	payoutsErr          error
}

func (f *fakeReportSource) DataRange(context.Context) (time.Time, time.Time, time.Time, error) {
	return f.rangeFirst, f.rangeLast, f.rangeActivity, f.rangeErr
}

func (f *fakeReportSource) Receivable(context.Context) (float64, error) {
	return f.receivable, f.receivableErr
}

func (f *fakeReportSource) MonthlyFacts(context.Context, time.Time, time.Time) ([]domain.MonthFacts, error) {
	return f.facts, f.factsErr
}

func (f *fakeReportSource) BalanceBefore(_ context.Context, from time.Time) (float64, error) {
	f.openingCalls = append(f.openingCalls, from)
	return f.opening, f.openingErr
}

func (f *fakeReportSource) AdvocatePayouts(_ context.Context, _, _ time.Time) (map[string]float64, float64, error) {
	return f.payouts, f.payoutsUnattributed, f.payoutsErr
}

func (f *fakeReportSource) AdvocateCollections(context.Context, time.Time, time.Time) ([]domain.AdvocateCollection, error) {
	return f.collections, f.collectionsErr
}

// --- helpers ---

type stores struct {
	categories *fakeCategoryStore
	advocates  *fakeAdvocateRateStore
	expenses   *fakeExpenseStore
	rules      *fakeRuleStore
	income     *fakeIncomeStore
	report     *fakeReportSource
}

func newService(s *stores) *appfinance.Service {
	return appfinance.NewService(appfinance.Deps{
		Categories: s.categories,
		Advocates:  s.advocates,
		Expenses:   s.expenses,
		Rules:      s.rules,
		Income:     s.income,
		Report:     s.report,
	})
}

func newStores() *stores {
	return &stores{
		categories: &fakeCategoryStore{},
		advocates:  &fakeAdvocateRateStore{},
		expenses:   newFakeExpenseStore(),
		rules:      &fakeRuleStore{},
		income:     &fakeIncomeStore{},
		report:     &fakeReportSource{},
	}
}

// The generator only writes occurrences already due against the real clock, so
// tests aim at the first day of last month: every day-of-month in it is past.
func lastMonth() time.Time {
	now := time.Now()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return firstOfThisMonth.AddDate(0, -1, 0)
}

func dueRule(id string, mutate func(*domain.Rule)) domain.Rule {
	r := domain.Rule{
		ID:            id,
		Name:          "hosting",
		CategoryCode:  "website",
		Vendor:        "hostiq",
		PaymentMethod: domain.PaymentCard,
		Amount:        642,
		DayOfMonth:    5,
		ActiveFrom:    lastMonth().AddDate(-1, 0, 0),
		IsActive:      true,
	}
	if mutate != nil {
		mutate(&r)
	}
	return r
}

func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// --- RunAutoExpenses ---

// Re-running the generator for a month is the normal case (cron plus a manual
// click); a second insert of the same ref would be a double charge.
func TestRunAutoExpensesIsIdempotent(t *testing.T) {
	month := lastMonth()
	s := newStores()
	s.rules.rules = []domain.Rule{dueRule("hosting", nil), dueRule("ads", func(r *domain.Rule) {
		r.Name = "google ads"
		r.CategoryCode = "google_ads"
	})}
	s.report.collections = []domain.AdvocateCollection{
		{AdvocateID: "borzov", AdvocateName: "Борзов", Collected: 45000, CommissionPercent: 35},
	}
	svc := newService(s)

	first, err := svc.RunAutoExpenses(context.Background(), month, "cron")
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Recurring != 2 || first.Payouts != 1 || first.Skipped != 0 {
		t.Fatalf("first run = %+v, want recurring 2, payouts 1, skipped 0", first)
	}

	second, err := svc.RunAutoExpenses(context.Background(), month, "cron")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Recurring != 0 || second.Payouts != 0 {
		t.Errorf("second run = %+v, want nothing generated", second)
	}
	if second.Skipped != 3 {
		t.Errorf("second run skipped = %d, want 3", second.Skipped)
	}
	if second.Month != domain.MonthKey(month) {
		t.Errorf("month = %q, want %q", second.Month, domain.MonthKey(month))
	}

	if len(s.expenses.rows) != 3 {
		t.Fatalf("store holds %d rows, want 3 after two runs", len(s.expenses.rows))
	}
	for _, ref := range []string{
		domain.RecurringRef("hosting", month),
		domain.RecurringRef("ads", month),
		domain.AdvocatePayoutRef("borzov", month),
	} {
		if got := s.expenses.countByRef(ref); got != 1 {
			t.Errorf("rows for ref %q = %d, want exactly 1", ref, got)
		}
	}
}

// A payout is money to a person: draft always, whatever the rules in the same
// pass do, and always in the advocates category the report treats as direct cost.
func TestRunAutoExpensesAdvocatePayoutIsAlwaysDraft(t *testing.T) {
	month := lastMonth()
	s := newStores()
	s.rules.rules = []domain.Rule{dueRule("hosting", func(r *domain.Rule) { r.AutoPost = true })}
	s.report.collections = []domain.AdvocateCollection{
		{AdvocateID: "borzov", AdvocateName: "Борзов", Collected: 45000, CommissionPercent: 35},
		{AdvocateID: "popov", AdvocateName: "Попов", Collected: 12000},
	}

	got, err := newService(s).RunAutoExpenses(context.Background(), month, "cron")
	if err != nil {
		t.Fatalf("RunAutoExpenses: %v", err)
	}
	if got.Payouts != 1 {
		t.Fatalf("payouts = %d, want 1 — the zero-percent advocate must generate nothing", got.Payouts)
	}
	if _, ok := s.expenses.refToRow[domain.AdvocatePayoutRef("popov", month)]; ok {
		t.Errorf("advocate with CommissionPercent 0 got a payout row")
	}

	payout := s.expenses.rowByRef(t, domain.AdvocatePayoutRef("borzov", month))
	if payout.Status != domain.StatusDraft {
		t.Errorf("status = %q, want draft even next to an auto-post rule", payout.Status)
	}
	if payout.Origin != domain.OriginDerived {
		t.Errorf("origin = %q, want derived", payout.Origin)
	}
	if payout.CategoryCode != domain.CategoryAdvocates {
		t.Errorf("category = %q, want %q", payout.CategoryCode, domain.CategoryAdvocates)
	}
	if payout.Amount != 15750 {
		t.Errorf("amount = %v, want 15750 (35%% of 45000)", payout.Amount)
	}
	if payout.PaymentMethod != domain.PaymentCard {
		t.Errorf("payment method = %q, want card", payout.PaymentMethod)
	}
	if payout.Vendor != "Борзов" {
		t.Errorf("vendor = %q, want the advocate name", payout.Vendor)
	}
	if payout.CreatedBy != "cron" {
		t.Errorf("createdBy = %q, want cron", payout.CreatedBy)
	}
}

func TestRunAutoExpensesCarriesRuleFieldsAndAutoPost(t *testing.T) {
	month := lastMonth()
	s := newStores()
	s.rules.rules = []domain.Rule{
		dueRule("auto", func(r *domain.Rule) {
			r.AutoPost = true
			r.Name = "domain renewal"
			r.CategoryCode = "website"
			r.Vendor = "namecheap"
			r.PaymentMethod = domain.PaymentInvoice
			r.Amount = 300
		}),
		dueRule("manual-confirm", func(r *domain.Rule) {
			r.AutoPost = false
			r.Name = "copywriter"
			r.CategoryCode = "copywriting"
			r.Vendor = "Ольга"
			r.PaymentMethod = domain.PaymentCompany
			r.Amount = 8000
		}),
	}

	if _, err := newService(s).RunAutoExpenses(context.Background(), month, "cron"); err != nil {
		t.Fatalf("RunAutoExpenses: %v", err)
	}

	auto := s.expenses.rowByRef(t, domain.RecurringRef("auto", month))
	if auto.Status != domain.StatusPosted {
		t.Errorf("auto-post rule status = %q, want posted", auto.Status)
	}
	if auto.Origin != domain.OriginRecurring {
		t.Errorf("origin = %q, want recurring", auto.Origin)
	}
	if auto.CategoryCode != "website" || auto.Vendor != "namecheap" || auto.PaymentMethod != domain.PaymentInvoice {
		t.Errorf("rule fields not carried over: %+v", auto)
	}
	if auto.Description != "domain renewal" || auto.Amount != 300 || auto.RuleID != "auto" {
		t.Errorf("row = %+v, want the rule's name/amount/id", auto)
	}
	if !auto.SpentAt.Equal(time.Date(month.Year(), month.Month(), 5, 0, 0, 0, 0, month.Location())) {
		t.Errorf("spentAt = %v, want day 5 of %s", auto.SpentAt, domain.MonthKey(month))
	}

	draft := s.expenses.rowByRef(t, domain.RecurringRef("manual-confirm", month))
	if draft.Status != domain.StatusDraft {
		t.Errorf("non-auto-post rule status = %q, want draft", draft.Status)
	}
	if draft.Origin != domain.OriginRecurring {
		t.Errorf("origin = %q, want recurring", draft.Origin)
	}
	if draft.CategoryCode != "copywriting" || draft.Vendor != "Ольга" || draft.PaymentMethod != domain.PaymentCompany {
		t.Errorf("rule fields not carried over: %+v", draft)
	}
}

func TestRunAutoExpensesPropagatesStoreErrors(t *testing.T) {
	month := lastMonth()

	tests := []struct {
		name  string
		setup func(*stores)
	}{
		{
			name: "rules",
			setup: func(s *stores) {
				s.rules.listErr = errBoom
			},
		},
		{
			name: "advocate collections",
			setup: func(s *stores) {
				s.report.collectionsErr = errBoom
			},
		},
		{
			name: "insert",
			setup: func(s *stores) {
				s.rules.rules = []domain.Rule{dueRule("hosting", nil)}
				s.expenses.insertErr = errBoom
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStores()
			tc.setup(s)

			_, err := newService(s).RunAutoExpenses(context.Background(), month, "cron")
			if !errors.Is(err, errBoom) {
				t.Fatalf("err = %v, want it to wrap errBoom", err)
			}
			if len(s.expenses.rows) != 0 {
				t.Errorf("store holds %d rows, want none written on failure", len(s.expenses.rows))
			}
		})
	}
}

// --- expenses ---

// A manual row must never carry a generated row's idempotency key, otherwise
// the generator would treat that month as already paid.
func TestCreateExpenseForcesManualPostedWithoutExternalRef(t *testing.T) {
	s := newStores()

	got, err := newService(s).CreateExpense(context.Background(), domain.Expense{
		Amount:        1200,
		CategoryCode:  "website",
		PaymentMethod: domain.PaymentCash,
		Status:        domain.StatusDraft,
		Origin:        domain.OriginRecurring,
		ExternalRef:   domain.RecurringRef("hosting", lastMonth()),
		RuleID:        "hosting",
	})
	if err != nil {
		t.Fatalf("CreateExpense: %v", err)
	}
	if got.Status != domain.StatusPosted {
		t.Errorf("status = %q, want posted", got.Status)
	}
	if got.Origin != domain.OriginManual {
		t.Errorf("origin = %q, want manual", got.Origin)
	}
	if got.ExternalRef != "" {
		t.Errorf("externalRef = %q, want empty", got.ExternalRef)
	}
	if len(s.expenses.rows) != 1 {
		t.Fatalf("store holds %d rows, want 1", len(s.expenses.rows))
	}
	stored := s.expenses.rows[0]
	if stored.ExternalRef != "" || stored.Status != domain.StatusPosted || stored.Origin != domain.OriginManual {
		t.Errorf("stored row = %+v, want posted/manual with no external ref", stored)
	}
}

// Staff correct a generated payout's amount before confirming it; the edit must
// not promote the draft — that is ConfirmExpense's job.
func TestUpdateExpenseKeepsDraftStatus(t *testing.T) {
	s := newStores()
	inserted, err := s.expenses.InsertGenerated(context.Background(), domain.Expense{
		Amount:       15750,
		CategoryCode: domain.CategoryAdvocates,
		Status:       domain.StatusDraft,
		Origin:       domain.OriginDerived,
		ExternalRef:  domain.AdvocatePayoutRef("borzov", lastMonth()),
	})
	if err != nil || !inserted {
		t.Fatalf("seed InsertGenerated = %v, %v", inserted, err)
	}
	id := s.expenses.rows[0].ID

	err = newService(s).UpdateExpense(context.Background(), domain.Expense{
		ID:           id,
		Amount:       12000,
		CategoryCode: domain.CategoryAdvocates,
	})
	if err != nil {
		t.Fatalf("UpdateExpense: %v", err)
	}

	row := s.expenses.rows[0]
	if row.Amount != 12000 {
		t.Errorf("amount = %v, want the corrected 12000", row.Amount)
	}
	if row.Status != domain.StatusDraft {
		t.Errorf("status = %q, want it still draft after an edit", row.Status)
	}
	if row.Origin != domain.OriginDerived {
		t.Errorf("origin = %q, want derived", row.Origin)
	}
	if len(s.expenses.updateCalls) != 1 {
		t.Fatalf("update calls = %d, want 1", len(s.expenses.updateCalls))
	}
	if got := s.expenses.updateCalls[0].Status; got == domain.StatusPosted {
		t.Errorf("service handed status %q to the store, want it left alone", got)
	}
}

func TestCreateExpenseDefaults(t *testing.T) {
	s := newStores()
	svc := newService(s)

	got, err := svc.CreateExpense(context.Background(), domain.Expense{
		Amount:       500,
		CategoryCode: "  website  ",
		Vendor:       "  hostiq\t",
		Description:  "  monthly hosting  ",
	})
	if err != nil {
		t.Fatalf("CreateExpense: %v", err)
	}
	if want := dayOf(time.Now()); !got.SpentAt.Equal(want) {
		t.Errorf("spentAt = %v, want today at midnight (%v)", got.SpentAt, want)
	}
	if got.PaymentMethod != domain.PaymentCard {
		t.Errorf("payment method = %q, want the card default", got.PaymentMethod)
	}
	if got.CategoryCode != "website" || got.Vendor != "hostiq" || got.Description != "monthly hosting" {
		t.Errorf("row = %+v, want category/vendor/description trimmed", got)
	}

	withClock, err := svc.CreateExpense(context.Background(), domain.Expense{
		Amount:       500,
		CategoryCode: "website",
		SpentAt:      time.Date(2026, time.March, 3, 15, 4, 5, 99, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateExpense with clock time: %v", err)
	}
	if want := time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC); !withClock.SpentAt.Equal(want) {
		t.Errorf("spentAt = %v, want it truncated to %v", withClock.SpentAt, want)
	}
}

// --- validation ---

// Every sentinel is checked with errors.Is: the service wraps with context, so
// an equality check would pass today and break on the next wrap.
func TestValidationSentinels(t *testing.T) {
	ctx := context.Background()
	svc := newService(newStores())

	validRule := func(mutate func(*domain.Rule)) domain.Rule {
		r := domain.Rule{Name: "hosting", CategoryCode: "website", Amount: 642, DayOfMonth: 5}
		mutate(&r)
		return r
	}

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "zero amount",
			call: func() error {
				_, err := svc.CreateExpense(ctx, domain.Expense{Amount: 0, CategoryCode: "website"})
				return err
			},
			want: domain.ErrInvalidAmount,
		},
		{
			name: "negative amount",
			call: func() error {
				_, err := svc.CreateExpense(ctx, domain.Expense{Amount: -1, CategoryCode: "website"})
				return err
			},
			want: domain.ErrInvalidAmount,
		},
		{
			name: "empty category",
			call: func() error {
				_, err := svc.CreateExpense(ctx, domain.Expense{Amount: 100, CategoryCode: "   "})
				return err
			},
			want: domain.ErrUnknownCategory,
		},
		{
			name: "unknown payment method",
			call: func() error {
				_, err := svc.CreateExpense(ctx, domain.Expense{
					Amount:        100,
					CategoryCode:  "website",
					PaymentMethod: domain.PaymentMethod("bitcoin"),
				})
				return err
			},
			want: domain.ErrInvalidPaymentMethod,
		},
		{
			name: "update expense keeps the same validation",
			call: func() error {
				return svc.UpdateExpense(ctx, domain.Expense{ID: "x", Amount: 0, CategoryCode: "website"})
			},
			want: domain.ErrInvalidAmount,
		},
		{
			name: "rule day zero",
			call: func() error {
				_, err := svc.CreateRule(ctx, validRule(func(r *domain.Rule) { r.DayOfMonth = 0 }))
				return err
			},
			want: domain.ErrInvalidDayOfMonth,
		},
		{
			name: "rule day 29",
			call: func() error {
				_, err := svc.CreateRule(ctx, validRule(func(r *domain.Rule) { r.DayOfMonth = 29 }))
				return err
			},
			want: domain.ErrInvalidDayOfMonth,
		},
		{
			name: "rule empty category",
			call: func() error {
				_, err := svc.CreateRule(ctx, validRule(func(r *domain.Rule) { r.CategoryCode = " " }))
				return err
			},
			want: domain.ErrUnknownCategory,
		},
		{
			name: "negative percent",
			call: func() error { return svc.SetAdvocateRate(ctx, "borzov", -0.5) },
			want: domain.ErrInvalidPercent,
		},
		{
			name: "percent above 100",
			call: func() error { return svc.SetAdvocateRate(ctx, "borzov", 100.5) },
			want: domain.ErrInvalidPercent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want it to wrap %v", err, tc.want)
			}
		})
	}
}

// 0 (no automatic payout) and 100 are legitimate rates, not off-by-one rejects.
func TestSetAdvocateRateAcceptsBounds(t *testing.T) {
	s := newStores()
	svc := newService(s)

	for _, percent := range []float64{0, 35, 100} {
		if err := svc.SetAdvocateRate(context.Background(), "borzov", percent); err != nil {
			t.Fatalf("SetAdvocateRate(%v): %v", percent, err)
		}
	}
	if len(s.advocates.set) != 3 {
		t.Errorf("store got %d calls, want 3", len(s.advocates.set))
	}
}

// --- report ---

// Opening balance keeps the running total continuous when the window starts
// mid-history, and the report needs retired categories too — their spend still
// belongs to a kind total.
func TestReportUsesOpeningBalanceAndAllCategories(t *testing.T) {
	s := newStores()
	s.categories.categories = []domain.Category{
		{Code: "google_ads", Kind: domain.KindMarketing, IsActive: true},
		{Code: "facebook_ads", Kind: domain.KindMarketing, IsActive: false},
	}
	s.report.opening = -100000
	s.report.facts = []domain.MonthFacts{{
		Month:             "2026-08",
		ConsultRevenue:    50000,
		ExpenseByCategory: map[string]float64{"google_ads": 20000, "facebook_ads": 5000},
	}}
	from := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC)

	got, err := newService(s).Report(context.Background(), from, to)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(got.Months) != 1 {
		t.Fatalf("months = %d, want 1", len(got.Months))
	}
	m := got.Months[0]
	if m.Balance != 25000 {
		t.Errorf("balance = %v, want 25000", m.Balance)
	}
	if m.Cumulative != -75000 {
		t.Errorf("cumulative = %v, want -75000 — the opening balance must carry into the running total", m.Cumulative)
	}

	if len(s.categories.activeOnlyCalls) != 1 {
		t.Fatalf("ListCategories calls = %v, want exactly one", s.categories.activeOnlyCalls)
	}
	if s.categories.activeOnlyCalls[0] {
		t.Errorf("ListCategories(activeOnly=true), want all categories so retired ones keep their kind")
	}
	if m.MarketingSpend != 25000 {
		t.Errorf("marketing = %v, want 25000 including the retired facebook_ads", m.MarketingSpend)
	}
	if len(s.report.openingCalls) != 1 || !s.report.openingCalls[0].Equal(from) {
		t.Errorf("BalanceBefore calls = %v, want one for %v", s.report.openingCalls, from)
	}
}

func TestReportPropagatesErrors(t *testing.T) {
	from := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1)

	tests := []struct {
		name  string
		setup func(*stores)
	}{
		{name: "facts", setup: func(s *stores) { s.report.factsErr = errBoom }},
		{name: "categories", setup: func(s *stores) { s.categories.listErr = errBoom }},
		{name: "opening balance", setup: func(s *stores) { s.report.openingErr = errBoom }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStores()
			tc.setup(s)
			if _, err := newService(s).Report(context.Background(), from, to); !errors.Is(err, errBoom) {
				t.Errorf("err = %v, want it to wrap errBoom", err)
			}
		})
	}
}

// A hard delete would free the row's external ref and the next generator pass
// would re-create the very expense staff removed, so generated rows are voided.
func TestDeleteExpenseVoidsGeneratedRows(t *testing.T) {
	deps := newStores()
	svc := newService(deps)
	ctx := context.Background()

	deps.expenses.rows = []domain.Expense{
		{ID: "generated", Amount: 642, Status: domain.StatusPosted, Origin: domain.OriginRecurring, ExternalRef: "rule:r1:2026-08"},
		{ID: "manual", Amount: 300, Status: domain.StatusPosted, Origin: domain.OriginManual},
	}

	if err := svc.DeleteExpense(ctx, "generated"); err != nil {
		t.Fatalf("delete generated: %v", err)
	}
	if err := svc.DeleteExpense(ctx, "manual"); err != nil {
		t.Fatalf("delete manual: %v", err)
	}

	if len(deps.expenses.rows) != 1 {
		t.Fatalf("want the generated row kept and the manual one gone, got %+v", deps.expenses.rows)
	}
	kept := deps.expenses.rows[0]
	if kept.ID != "generated" {
		t.Fatalf("kept row = %q, want the generated one", kept.ID)
	}
	if kept.Status != domain.StatusVoid {
		t.Errorf("status = %q, want void", kept.Status)
	}
	if kept.ExternalRef == "" {
		t.Error("external ref was cleared — the generator would re-create this row")
	}
}

func TestDeleteExpenseUnknownID(t *testing.T) {
	svc := newService(newStores())
	if err := svc.DeleteExpense(context.Background(), "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// The page asks for the span the data covers because a window relative to today
// showed twelve empty columns; an empty database must say so rather than
// pretending the current month is meaningful.
func TestPeriodReportsWhatTheDataSpans(t *testing.T) {
	deps := newStores()
	deps.report.rangeFirst = time.Date(2024, time.June, 5, 0, 0, 0, 0, time.UTC)
	deps.report.rangeLast = time.Date(2025, time.April, 12, 0, 0, 0, 0, time.UTC)
	// leads keep arriving long after the last payment
	deps.report.rangeActivity = time.Date(2026, time.August, 7, 0, 0, 0, 0, time.UTC)
	svc := newService(deps)

	got, err := svc.Period(context.Background())
	if err != nil {
		t.Fatalf("Period: %v", err)
	}
	if !got.HasData || got.FirstMonth != "2024-06" || got.LastMonth != "2025-04" {
		t.Fatalf("got %+v, want 2024-06..2025-04 with data", got)
	}
	if got.LastActivityMonth != "2026-08" {
		t.Errorf("last activity = %q, want 2026-08 — lead months stay selectable", got.LastActivityMonth)
	}

	empty := newStores()
	if got, err := newService(empty).Period(context.Background()); err != nil || got.HasData {
		t.Fatalf("empty database: got %+v, %v; want no data and no error", got, err)
	}
}
