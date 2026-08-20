package finance

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	domain "multiagent-seo/internal/domain/finance"
)

const defaultLimit = 50

type Deps struct {
	Categories domain.CategoryStore
	Advocates  domain.AdvocateRateStore
	Expenses   domain.ExpenseStore
	Rules      domain.RuleStore
	Income     domain.IncomeStore
	Report     domain.ReportSource
	Log        *slog.Logger
}

type Service struct {
	categories domain.CategoryStore
	advocates  domain.AdvocateRateStore
	expenses   domain.ExpenseStore
	rules      domain.RuleStore
	income     domain.IncomeStore
	report     domain.ReportSource
	log        *slog.Logger
}

func NewService(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		categories: d.Categories,
		advocates:  d.Advocates,
		expenses:   d.Expenses,
		rules:      d.Rules,
		income:     d.Income,
		report:     d.Report,
		log:        log,
	}
}

func (s *Service) ListCategories(ctx context.Context, activeOnly bool) ([]domain.Category, error) {
	list, err := s.categories.ListCategories(ctx, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("finance: list categories: %w", err)
	}
	return list, nil
}

func (s *Service) CreateCategory(ctx context.Context, c domain.Category) error {
	c.Code = strings.TrimSpace(strings.ToLower(c.Code))
	c.Label = strings.TrimSpace(c.Label)
	if c.Code == "" || c.Label == "" {
		return fmt.Errorf("finance: create category: %w", domain.ErrInvalidName)
	}
	if !domain.IsKind(c.Kind) {
		return fmt.Errorf("finance: create category: %w: %q", domain.ErrInvalidKind, c.Kind)
	}
	if err := s.categories.CreateCategory(ctx, c); err != nil {
		return fmt.Errorf("finance: create category %q: %w", c.Code, err)
	}
	return nil
}

func (s *Service) UpdateCategory(ctx context.Context, c domain.Category) error {
	c.Label = strings.TrimSpace(c.Label)
	if c.Label == "" {
		return fmt.Errorf("finance: update category: %w", domain.ErrInvalidName)
	}
	if !domain.IsKind(c.Kind) {
		return fmt.Errorf("finance: update category: %w: %q", domain.ErrInvalidKind, c.Kind)
	}
	if err := s.categories.UpdateCategory(ctx, c); err != nil {
		return fmt.Errorf("finance: update category %q: %w", c.Code, err)
	}
	return nil
}

func (s *Service) ListExpenses(ctx context.Context, filter domain.ExpenseFilter) (domain.ExpenseList, error) {
	if filter.Limit <= 0 {
		filter.Limit = defaultLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.Search = strings.TrimSpace(filter.Search)
	list, err := s.expenses.ListExpenses(ctx, filter)
	if err != nil {
		return domain.ExpenseList{}, fmt.Errorf("finance: list expenses: %w", err)
	}
	return list, nil
}

// Manual entry is always posted — a person typing a spend they already made
// has nothing left to confirm; drafts exist only for generated rows.
func (s *Service) CreateExpense(ctx context.Context, e domain.Expense) (domain.Expense, error) {
	e, err := s.normalizeExpense(e)
	if err != nil {
		return domain.Expense{}, fmt.Errorf("finance: create expense: %w", err)
	}
	e.Status = domain.StatusPosted
	e.Origin = domain.OriginManual
	e.ExternalRef = ""
	e.RuleID = ""

	created, err := s.expenses.CreateExpense(ctx, e)
	if err != nil {
		return domain.Expense{}, fmt.Errorf("finance: create expense: %w", err)
	}
	return created, nil
}

// Editing keeps the row's status: correcting a draft's amount before
// confirming it is the normal flow, and must not silently post it.
func (s *Service) UpdateExpense(ctx context.Context, e domain.Expense) error {
	e, err := s.normalizeExpense(e)
	if err != nil {
		return fmt.Errorf("finance: update expense: %w", err)
	}
	if err := s.expenses.UpdateExpense(ctx, e); err != nil {
		return fmt.Errorf("finance: update expense %q: %w", e.ID, err)
	}
	return nil
}

func (s *Service) ConfirmExpense(ctx context.Context, id string) error {
	if err := s.expenses.ConfirmExpense(ctx, id); err != nil {
		return fmt.Errorf("finance: confirm expense %q: %w", id, err)
	}
	return nil
}

func (s *Service) DeleteExpense(ctx context.Context, id string) error {
	existing, err := s.expenses.GetExpense(ctx, id)
	if err != nil {
		return fmt.Errorf("finance: delete expense %q: %w", id, err)
	}
	// A generated row is voided, not deleted: deleting frees its ExternalRef, and
	// the next generator pass re-creates the very expense staff just removed.
	if existing.ExternalRef != "" {
		if err := s.expenses.VoidExpense(ctx, id); err != nil {
			return fmt.Errorf("finance: void expense %q: %w", id, err)
		}
		return nil
	}
	if err := s.expenses.DeleteExpense(ctx, id); err != nil {
		return fmt.Errorf("finance: delete expense %q: %w", id, err)
	}
	return nil
}

func (s *Service) normalizeExpense(e domain.Expense) (domain.Expense, error) {
	e.CategoryCode = strings.TrimSpace(e.CategoryCode)
	e.Vendor = strings.TrimSpace(e.Vendor)
	e.Description = strings.TrimSpace(e.Description)
	if e.CategoryCode == "" {
		return domain.Expense{}, domain.ErrUnknownCategory
	}
	if e.Amount <= 0 {
		return domain.Expense{}, fmt.Errorf("%w: %v", domain.ErrInvalidAmount, e.Amount)
	}
	if e.PaymentMethod == "" {
		e.PaymentMethod = domain.PaymentCard
	}
	if !domain.IsPaymentMethod(e.PaymentMethod) {
		return domain.Expense{}, fmt.Errorf("%w: %q", domain.ErrInvalidPaymentMethod, e.PaymentMethod)
	}
	if e.SpentAt.IsZero() {
		e.SpentAt = time.Now()
	}
	e.SpentAt = dayOf(e.SpentAt)
	return e, nil
}

func (s *Service) ListRules(ctx context.Context, activeOnly bool) ([]domain.Rule, error) {
	list, err := s.rules.ListRules(ctx, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("finance: list rules: %w", err)
	}
	return list, nil
}

func (s *Service) CreateRule(ctx context.Context, r domain.Rule) (domain.Rule, error) {
	r, err := s.normalizeRule(r)
	if err != nil {
		return domain.Rule{}, fmt.Errorf("finance: create rule: %w", err)
	}
	created, err := s.rules.CreateRule(ctx, r)
	if err != nil {
		return domain.Rule{}, fmt.Errorf("finance: create rule: %w", err)
	}
	return created, nil
}

func (s *Service) UpdateRule(ctx context.Context, r domain.Rule) error {
	r, err := s.normalizeRule(r)
	if err != nil {
		return fmt.Errorf("finance: update rule: %w", err)
	}
	if err := s.rules.UpdateRule(ctx, r); err != nil {
		return fmt.Errorf("finance: update rule %q: %w", r.ID, err)
	}
	return nil
}

func (s *Service) DeleteRule(ctx context.Context, id string) error {
	if err := s.rules.DeleteRule(ctx, id); err != nil {
		return fmt.Errorf("finance: delete rule %q: %w", id, err)
	}
	return nil
}

func (s *Service) normalizeRule(r domain.Rule) (domain.Rule, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.CategoryCode = strings.TrimSpace(r.CategoryCode)
	r.Vendor = strings.TrimSpace(r.Vendor)
	if r.Name == "" {
		return domain.Rule{}, domain.ErrInvalidName
	}
	if r.CategoryCode == "" {
		return domain.Rule{}, domain.ErrUnknownCategory
	}
	if r.Amount <= 0 {
		return domain.Rule{}, fmt.Errorf("%w: %v", domain.ErrInvalidAmount, r.Amount)
	}
	if r.DayOfMonth < 1 || r.DayOfMonth > 28 {
		return domain.Rule{}, fmt.Errorf("%w: %d", domain.ErrInvalidDayOfMonth, r.DayOfMonth)
	}
	if r.PaymentMethod == "" {
		r.PaymentMethod = domain.PaymentCard
	}
	if !domain.IsPaymentMethod(r.PaymentMethod) {
		return domain.Rule{}, fmt.Errorf("%w: %q", domain.ErrInvalidPaymentMethod, r.PaymentMethod)
	}
	if r.ActiveFrom.IsZero() {
		r.ActiveFrom = time.Now()
	}
	r.ActiveFrom = dayOf(r.ActiveFrom)
	if r.ActiveTo != nil {
		to := dayOf(*r.ActiveTo)
		r.ActiveTo = &to
	}
	return r, nil
}

func (s *Service) ListOtherIncome(ctx context.Context, from, to time.Time) ([]domain.OtherIncome, error) {
	list, err := s.income.ListOtherIncome(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("finance: list other income: %w", err)
	}
	return list, nil
}

func (s *Service) CreateOtherIncome(ctx context.Context, i domain.OtherIncome) (domain.OtherIncome, error) {
	i.Source = strings.TrimSpace(i.Source)
	i.Description = strings.TrimSpace(i.Description)
	if i.Amount <= 0 {
		return domain.OtherIncome{}, fmt.Errorf("finance: create other income: %w: %v", domain.ErrInvalidAmount, i.Amount)
	}
	if i.ReceivedAt.IsZero() {
		i.ReceivedAt = time.Now()
	}
	i.ReceivedAt = dayOf(i.ReceivedAt)

	created, err := s.income.CreateOtherIncome(ctx, i)
	if err != nil {
		return domain.OtherIncome{}, fmt.Errorf("finance: create other income: %w", err)
	}
	return created, nil
}

func (s *Service) DeleteOtherIncome(ctx context.Context, id string) error {
	if err := s.income.DeleteOtherIncome(ctx, id); err != nil {
		return fmt.Errorf("finance: delete other income %q: %w", id, err)
	}
	return nil
}

func (s *Service) ListAdvocateRates(ctx context.Context) ([]domain.AdvocateRate, error) {
	list, err := s.advocates.ListAdvocateRates(ctx)
	if err != nil {
		return nil, fmt.Errorf("finance: list advocate rates: %w", err)
	}
	return list, nil
}

func (s *Service) SetAdvocateRate(ctx context.Context, advocateID string, percent float64) error {
	if percent < 0 || percent > 100 {
		return fmt.Errorf("finance: set advocate rate: %w: %v", domain.ErrInvalidPercent, percent)
	}
	if err := s.advocates.SetAdvocateRate(ctx, advocateID, percent); err != nil {
		return fmt.Errorf("finance: set advocate rate %q: %w", advocateID, err)
	}
	return nil
}

func (s *Service) Report(ctx context.Context, from, to time.Time) (domain.Report, error) {
	// MonthlyFacts reports whole months, so the opening balance has to be cut at
	// the same boundary — a mid-month `from` would otherwise count that month's
	// earlier rows both in the opening balance and in the month's own row.
	from = startOfMonth(from)

	facts, err := s.report.MonthlyFacts(ctx, from, to)
	if err != nil {
		return domain.Report{}, fmt.Errorf("finance: monthly facts: %w", err)
	}
	// every category, not just active ones — a deactivated category still has
	// history, and dropping it would leave its spend out of the kind totals
	categories, err := s.categories.ListCategories(ctx, false)
	if err != nil {
		return domain.Report{}, fmt.Errorf("finance: report categories: %w", err)
	}
	opening, err := s.report.BalanceBefore(ctx, from)
	if err != nil {
		return domain.Report{}, fmt.Errorf("finance: opening balance: %w", err)
	}
	return domain.BuildReport(facts, categories, opening), nil
}

// RunAutoExpenses materializes the recurring templates and advocate payouts
// due in month. Safe to call repeatedly: every generated row carries an
// external ref the store inserts on-conflict-do-nothing.
func (s *Service) RunAutoExpenses(ctx context.Context, month time.Time, createdBy string) (domain.Generated, error) {
	// Dates from the database are UTC midnight, so planning is done in UTC too:
	// the cron passes a local time.Now() while the HTTP path parses "2006-01" as
	// UTC, and without this the two entry points disagree on which day is due.
	month = startOfMonth(month.UTC())
	now := time.Now().UTC()

	out := domain.Generated{Month: domain.MonthKey(month)}

	rules, err := s.rules.ListRules(ctx, true)
	if err != nil {
		return out, fmt.Errorf("finance: run auto expenses: list rules: %w", err)
	}
	for _, p := range domain.PlanRecurring(rules, month, now) {
		inserted, err := s.expenses.InsertGenerated(ctx, domain.Expense{
			SpentAt:       p.SpentAt,
			Amount:        p.Amount,
			CategoryCode:  p.Rule.CategoryCode,
			PaymentMethod: p.Rule.PaymentMethod,
			Vendor:        p.Rule.Vendor,
			Description:   p.Rule.Name,
			Status:        p.Status,
			Origin:        domain.OriginRecurring,
			RuleID:        p.Rule.ID,
			ExternalRef:   p.ExternalRef,
			CreatedBy:     createdBy,
		})
		if err != nil {
			return out, fmt.Errorf("finance: run auto expenses: rule %q: %w", p.Rule.ID, err)
		}
		if inserted {
			out.Recurring++
		} else {
			out.Skipped++
		}
	}

	collections, err := s.report.AdvocateCollections(ctx, month)
	if err != nil {
		return out, fmt.Errorf("finance: run auto expenses: advocate collections: %w", err)
	}
	for _, p := range domain.PlanAdvocatePayouts(collections, month, now) {
		inserted, err := s.expenses.InsertGenerated(ctx, domain.Expense{
			SpentAt:      p.SpentAt,
			Amount:       p.Amount,
			CategoryCode: domain.CategoryAdvocates,
			// a payout to a person is never auto-posted, however confident the percentage is
			Status:        domain.StatusDraft,
			Origin:        domain.OriginDerived,
			PaymentMethod: domain.PaymentCard,
			Vendor:        p.AdvocateName,
			Description:   fmt.Sprintf("%s: %g%% от собранных %.0f ₴", p.AdvocateName, p.Percent, p.Collected),
			ExternalRef:   p.ExternalRef,
			CreatedBy:     createdBy,
		})
		if err != nil {
			return out, fmt.Errorf("finance: run auto expenses: advocate %q: %w", p.AdvocateID, err)
		}
		if inserted {
			out.Payouts++
		} else {
			out.Skipped++
		}
	}

	return out, nil
}

// GenerateDueExpenses is the cron entry point — the outermost layer for this
// path, so it logs instead of returning the error to nobody.
func (s *Service) GenerateDueExpenses(ctx context.Context) {
	now := time.Now().UTC()
	generated, err := s.RunAutoExpenses(ctx, now, "cron")
	if err != nil {
		s.log.ErrorContext(ctx, "finance: auto expenses failed", "month", domain.MonthKey(now), "err", err)
		return
	}

	// Advocate payouts are only planned for a month that has ended, so the pass
	// above never produces them — the previous month is what closes them out.
	previous := startOfMonth(now).AddDate(0, -1, 0)
	closed, err := s.RunAutoExpenses(ctx, previous, "cron")
	if err != nil {
		s.log.ErrorContext(ctx, "finance: auto expenses failed", "month", domain.MonthKey(previous), "err", err)
		return
	}
	generated.Recurring += closed.Recurring
	generated.Payouts += closed.Payouts
	generated.Skipped += closed.Skipped

	if generated.Recurring == 0 && generated.Payouts == 0 {
		s.log.DebugContext(ctx, "finance: auto expenses, nothing new",
			"month", generated.Month,
			"skipped", generated.Skipped,
		)
		return
	}
	s.log.InfoContext(ctx, "finance: auto expenses generated",
		"month", generated.Month,
		"recurring", generated.Recurring,
		"payouts", generated.Payouts,
		"skipped", generated.Skipped,
	)
}

func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}
