package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	domain "multiagent-seo/internal/domain/finance"
	"multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

type financeService interface {
	ListCategories(ctx context.Context, activeOnly bool) ([]domain.Category, error)
	CreateCategory(ctx context.Context, c domain.Category) error
	UpdateCategory(ctx context.Context, c domain.Category) error
	ListExpenses(ctx context.Context, filter domain.ExpenseFilter) (domain.ExpenseList, error)
	CreateExpense(ctx context.Context, e domain.Expense) (domain.Expense, error)
	UpdateExpense(ctx context.Context, e domain.Expense) error
	ConfirmExpense(ctx context.Context, id string) error
	DeleteExpense(ctx context.Context, id string) error
	ListRules(ctx context.Context, activeOnly bool) ([]domain.Rule, error)
	CreateRule(ctx context.Context, r domain.Rule) (domain.Rule, error)
	UpdateRule(ctx context.Context, r domain.Rule) error
	DeleteRule(ctx context.Context, id string) error
	ListOtherIncome(ctx context.Context, from, to time.Time) ([]domain.OtherIncome, error)
	CreateOtherIncome(ctx context.Context, i domain.OtherIncome) (domain.OtherIncome, error)
	DeleteOtherIncome(ctx context.Context, id string) error
	ListAdvocateRates(ctx context.Context) ([]domain.AdvocateRate, error)
	SetAdvocateRate(ctx context.Context, advocateID string, percent float64) error
	Period(ctx context.Context) (domain.Period, error)
	Gaps(ctx context.Context, from, to time.Time) ([]domain.DataGap, error)
	Settlement(ctx context.Context, from, to time.Time) (domain.Settlement, error)
	Report(ctx context.Context, from, to time.Time) (domain.Report, error)
	RunAutoExpenses(ctx context.Context, month time.Time, createdBy string) (domain.Generated, error)
}

type FinanceHandler struct {
	svc financeService
}

func NewFinanceHandler(svc financeService) *FinanceHandler {
	return &FinanceHandler{svc: svc}
}

func (h *FinanceHandler) GetFinanceReport(w http.ResponseWriter, r *http.Request, params oapigen.GetFinanceReportParams) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}
	from, to := params.From.Time, params.To.Time
	if to.Before(from) {
		problem.Write(w, http.StatusBadRequest, "to is before from")
		return
	}

	report, err := h.svc.Report(r.Context(), from, to)
	if err != nil {
		h.writeError(r.Context(), w, "finance_report", err)
		return
	}

	months := make([]oapigen.FinanceMonth, len(report.Months))
	for i, m := range report.Months {
		months[i] = toAPIFinanceMonth(m)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.FinanceReport{
		Months:     months,
		Total:      toAPIFinanceMonth(report.Total),
		Receivable: report.Receivable,
	})
}

func (h *FinanceHandler) GetFinanceGaps(w http.ResponseWriter, r *http.Request, params oapigen.GetFinanceGapsParams) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}
	from, to := params.From.Time, params.To.Time
	if to.Before(from) {
		problem.Write(w, http.StatusBadRequest, "to is before from")
		return
	}

	gaps, err := h.svc.Gaps(r.Context(), from, to)
	if err != nil {
		h.writeError(r.Context(), w, "finance_gaps", err)
		return
	}

	items := make([]oapigen.DataGap, len(gaps))
	for i, g := range gaps {
		items[i] = oapigen.DataGap{Kind: g.Kind, Count: g.Count, Amount: g.Amount}
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.DataGapList{Items: items})
}

func (h *FinanceHandler) GetFinancePeriod(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	period, err := h.svc.Period(r.Context())
	if err != nil {
		h.writeError(r.Context(), w, "finance_period", err)
		return
	}

	out := oapigen.FinancePeriod{HasData: period.HasData}
	if period.HasData {
		out.FirstMonth = &period.FirstMonth
		out.LastMonth = &period.LastMonth
		out.LastActivityMonth = &period.LastActivityMonth
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *FinanceHandler) GetAdvocateSettlement(w http.ResponseWriter, r *http.Request, params oapigen.GetAdvocateSettlementParams) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}
	from, to := params.From.Time, params.To.Time
	if to.Before(from) {
		problem.Write(w, http.StatusBadRequest, "to is before from")
		return
	}

	settlement, err := h.svc.Settlement(r.Context(), from, to)
	if err != nil {
		h.writeError(r.Context(), w, "advocate_settlement", err)
		return
	}

	items := make([]oapigen.AdvocateSettlement, len(settlement.Advocates))
	for i, a := range settlement.Advocates {
		items[i] = oapigen.AdvocateSettlement{
			AdvocateId:        a.AdvocateID,
			FullName:          a.FullName,
			CommissionPercent: a.CommissionPercent,
			Collected:         a.Collected,
			Accrued:           a.Accrued,
			Paid:              a.Paid,
			Outstanding:       a.Outstanding,
		}
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.AdvocateSettlementList{
		Items:            items,
		UnattributedPaid: settlement.UnattributedPaid,
		ConsultIncome:    settlement.ConsultIncome,
		CaseIncome:       settlement.CaseIncome,
		TotalAccrued:     settlement.TotalAccrued,
		TotalPaid:        settlement.TotalPaid,
		TotalOutstanding: settlement.TotalOutstanding,
	})
}

func (h *FinanceHandler) ListExpenses(w http.ResponseWriter, r *http.Request, params oapigen.ListExpensesParams) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	filter := domain.ExpenseFilter{}
	if params.From != nil {
		filter.From = params.From.Time
	}
	if params.To != nil {
		filter.To = params.To.Time
	}
	if params.Category != nil {
		filter.CategoryCode = *params.Category
	}
	if params.Status != nil {
		filter.Status = domain.Status(*params.Status)
	}
	if params.Origin != nil {
		filter.Origin = domain.Origin(*params.Origin)
	}
	if params.PaymentMethod != nil {
		filter.PaymentMethod = domain.PaymentMethod(*params.PaymentMethod)
	}
	if params.Search != nil {
		filter.Search = *params.Search
	}
	if params.Limit != nil {
		filter.Limit = *params.Limit
	}
	if params.Offset != nil {
		filter.Offset = *params.Offset
	}

	list, err := h.svc.ListExpenses(r.Context(), filter)
	if err != nil {
		h.writeError(r.Context(), w, "list_expenses", err)
		return
	}

	items := make([]oapigen.Expense, len(list.Items))
	for i, e := range list.Items {
		items[i] = toAPIExpense(e)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.ExpenseList{
		Items: items,
		Total: int64(list.Total),
		Sum:   list.Sum,
	})
}

func (h *FinanceHandler) CreateExpense(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	var body oapigen.CreateExpenseRequest
	if !h.decode(w, r, &body, "create expense") {
		return
	}

	createdBy, _ := middleware.UserIDFromContext(r.Context())
	created, err := h.svc.CreateExpense(r.Context(), domain.Expense{
		SpentAt:       body.SpentAt.Time,
		Amount:        body.Amount,
		CategoryCode:  body.CategoryCode,
		PaymentMethod: paymentMethodOf(body.PaymentMethod),
		Vendor:        strOf(body.Vendor),
		Description:   strOf(body.Description),
		CreatedBy:     createdBy,
	})
	if err != nil {
		h.writeError(r.Context(), w, "create_expense", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, toAPIExpense(created))
}

func (h *FinanceHandler) UpdateExpense(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	var body oapigen.UpdateExpenseRequest
	if !h.decode(w, r, &body, "update expense") {
		return
	}

	err := h.svc.UpdateExpense(r.Context(), domain.Expense{
		ID:            id.String(),
		SpentAt:       body.SpentAt.Time,
		Amount:        body.Amount,
		CategoryCode:  body.CategoryCode,
		PaymentMethod: paymentMethodOf(body.PaymentMethod),
		Vendor:        strOf(body.Vendor),
		Description:   strOf(body.Description),
	})
	if err != nil {
		h.writeError(r.Context(), w, "update_expense", err)
		return
	}
	response.NoContent(w)
}

func (h *FinanceHandler) ConfirmExpense(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}
	if err := h.svc.ConfirmExpense(r.Context(), id.String()); err != nil {
		h.writeError(r.Context(), w, "confirm_expense", err)
		return
	}
	response.NoContent(w)
}

func (h *FinanceHandler) DeleteExpense(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}
	if err := h.svc.DeleteExpense(r.Context(), id.String()); err != nil {
		h.writeError(r.Context(), w, "delete_expense", err)
		return
	}
	response.NoContent(w)
}

func (h *FinanceHandler) RunAutoExpenses(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	// body is optional here — no body at all (io.EOF) means "the current month",
	// but anything present and malformed is still a 400, chunked or not
	var body oapigen.RunAutoExpensesRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		log := logger.New(r.Context(), "handlers.finance")
		log.Debug().Err(err).Msg("decode run auto expenses body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}

	month := time.Now()
	if body.Month != nil {
		parsed, err := time.Parse("2006-01", *body.Month)
		if err != nil {
			problem.Write(w, http.StatusBadRequest, "month must be YYYY-MM")
			return
		}
		month = parsed
	}

	createdBy, _ := middleware.UserIDFromContext(r.Context())
	generated, err := h.svc.RunAutoExpenses(r.Context(), month, createdBy)
	if err != nil {
		h.writeError(r.Context(), w, "run_auto_expenses", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.GeneratedExpenses{
		Month:     generated.Month,
		Recurring: generated.Recurring,
		Payouts:   generated.Payouts,
		Skipped:   generated.Skipped,
	})
}

func (h *FinanceHandler) ListExpenseCategories(w http.ResponseWriter, r *http.Request, params oapigen.ListExpenseCategoriesParams) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}
	activeOnly := params.ActiveOnly != nil && *params.ActiveOnly

	list, err := h.svc.ListCategories(r.Context(), activeOnly)
	if err != nil {
		h.writeError(r.Context(), w, "list_expense_categories", err)
		return
	}
	items := make([]oapigen.ExpenseCategory, len(list))
	for i, c := range list {
		items[i] = oapigen.ExpenseCategory{
			Code:        c.Code,
			Label:       c.Label,
			Kind:        oapigen.ExpenseKind(c.Kind),
			IsPeoplePay: c.IsPeoplePay,
			IsActive:    c.IsActive,
			SortOrder:   c.SortOrder,
		}
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.ExpenseCategoryList{Items: items})
}

func (h *FinanceHandler) CreateExpenseCategory(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	var body oapigen.CreateExpenseCategoryRequest
	if !h.decode(w, r, &body, "create expense category") {
		return
	}

	c := domain.Category{
		Code:     body.Code,
		Label:    body.Label,
		Kind:     domain.Kind(body.Kind),
		IsActive: true,
	}
	if body.IsPeoplePay != nil {
		c.IsPeoplePay = *body.IsPeoplePay
	}
	if body.SortOrder != nil {
		c.SortOrder = *body.SortOrder
	}
	if err := h.svc.CreateCategory(r.Context(), c); err != nil {
		h.writeError(r.Context(), w, "create_expense_category", err)
		return
	}
	response.NoContent(w)
}

func (h *FinanceHandler) UpdateExpenseCategory(w http.ResponseWriter, r *http.Request, code string) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	var body oapigen.UpdateExpenseCategoryRequest
	if !h.decode(w, r, &body, "update expense category") {
		return
	}

	c := domain.Category{
		Code:     code,
		Label:    body.Label,
		Kind:     domain.Kind(body.Kind),
		IsActive: body.IsActive,
	}
	if body.IsPeoplePay != nil {
		c.IsPeoplePay = *body.IsPeoplePay
	}
	if body.SortOrder != nil {
		c.SortOrder = *body.SortOrder
	}
	if err := h.svc.UpdateCategory(r.Context(), c); err != nil {
		h.writeError(r.Context(), w, "update_expense_category", err)
		return
	}
	response.NoContent(w)
}

func (h *FinanceHandler) ListExpenseRules(w http.ResponseWriter, r *http.Request, params oapigen.ListExpenseRulesParams) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}
	activeOnly := params.ActiveOnly != nil && *params.ActiveOnly

	list, err := h.svc.ListRules(r.Context(), activeOnly)
	if err != nil {
		h.writeError(r.Context(), w, "list_expense_rules", err)
		return
	}
	items := make([]oapigen.ExpenseRule, len(list))
	for i, rule := range list {
		items[i] = toAPIExpenseRule(rule)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.ExpenseRuleList{Items: items})
}

func (h *FinanceHandler) CreateExpenseRule(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	var body oapigen.CreateExpenseRuleRequest
	if !h.decode(w, r, &body, "create expense rule") {
		return
	}

	createdBy, _ := middleware.UserIDFromContext(r.Context())
	rule := ruleFromRequest(body)
	rule.CreatedBy = createdBy

	created, err := h.svc.CreateRule(r.Context(), rule)
	if err != nil {
		h.writeError(r.Context(), w, "create_expense_rule", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, toAPIExpenseRule(created))
}

func (h *FinanceHandler) UpdateExpenseRule(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	var body oapigen.UpdateExpenseRuleRequest
	if !h.decode(w, r, &body, "update expense rule") {
		return
	}

	rule := ruleFromRequest(oapigen.CreateExpenseRuleRequest{
		Name:          body.Name,
		CategoryCode:  body.CategoryCode,
		Vendor:        body.Vendor,
		PaymentMethod: body.PaymentMethod,
		Amount:        body.Amount,
		DayOfMonth:    body.DayOfMonth,
		AutoPost:      &body.AutoPost,
		ActiveFrom:    &body.ActiveFrom,
		ActiveTo:      body.ActiveTo,
		IsActive:      &body.IsActive,
	})
	rule.ID = id.String()
	if err := h.svc.UpdateRule(r.Context(), rule); err != nil {
		h.writeError(r.Context(), w, "update_expense_rule", err)
		return
	}
	response.NoContent(w)
}

func (h *FinanceHandler) DeleteExpenseRule(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}
	if err := h.svc.DeleteRule(r.Context(), id.String()); err != nil {
		h.writeError(r.Context(), w, "delete_expense_rule", err)
		return
	}
	response.NoContent(w)
}

func (h *FinanceHandler) ListOtherIncome(w http.ResponseWriter, r *http.Request, params oapigen.ListOtherIncomeParams) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	list, err := h.svc.ListOtherIncome(r.Context(), params.From.Time, params.To.Time)
	if err != nil {
		h.writeError(r.Context(), w, "list_other_income", err)
		return
	}

	items := make([]oapigen.OtherIncome, len(list))
	var sum float64
	for i, in := range list {
		items[i] = oapigen.OtherIncome{
			Id:          in.ID,
			ReceivedAt:  openapi_types.Date{Time: in.ReceivedAt},
			Amount:      in.Amount,
			Source:      in.Source,
			Description: in.Description,
			CreatedAt:   in.CreatedAt,
		}
		sum += in.Amount
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.OtherIncomeList{Items: items, Sum: sum})
}

func (h *FinanceHandler) CreateOtherIncome(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	var body oapigen.CreateOtherIncomeRequest
	if !h.decode(w, r, &body, "create other income") {
		return
	}

	createdBy, _ := middleware.UserIDFromContext(r.Context())
	created, err := h.svc.CreateOtherIncome(r.Context(), domain.OtherIncome{
		ReceivedAt:  body.ReceivedAt.Time,
		Amount:      body.Amount,
		Source:      strOf(body.Source),
		Description: strOf(body.Description),
		CreatedBy:   createdBy,
	})
	if err != nil {
		h.writeError(r.Context(), w, "create_other_income", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, oapigen.OtherIncome{
		Id:          created.ID,
		ReceivedAt:  openapi_types.Date{Time: created.ReceivedAt},
		Amount:      created.Amount,
		Source:      created.Source,
		Description: created.Description,
		CreatedAt:   created.CreatedAt,
	})
}

func (h *FinanceHandler) DeleteOtherIncome(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}
	if err := h.svc.DeleteOtherIncome(r.Context(), id.String()); err != nil {
		h.writeError(r.Context(), w, "delete_other_income", err)
		return
	}
	response.NoContent(w)
}

func (h *FinanceHandler) ListAdvocateRates(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	list, err := h.svc.ListAdvocateRates(r.Context())
	if err != nil {
		h.writeError(r.Context(), w, "list_advocate_rates", err)
		return
	}
	items := make([]oapigen.AdvocateRate, len(list))
	for i, a := range list {
		items[i] = oapigen.AdvocateRate{
			AdvocateId:        a.AdvocateID,
			FullName:          a.FullName,
			IsActive:          a.IsActive,
			CommissionPercent: a.CommissionPercent,
		}
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.AdvocateRateList{Items: items})
}

func (h *FinanceHandler) SetAdvocateRate(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "finance unavailable")
		return
	}

	var body oapigen.SetAdvocateRateRequest
	if !h.decode(w, r, &body, "set advocate rate") {
		return
	}

	if err := h.svc.SetAdvocateRate(r.Context(), id.String(), body.CommissionPercent); err != nil {
		h.writeError(r.Context(), w, "set_advocate_rate", err)
		return
	}
	response.NoContent(w)
}

func (h *FinanceHandler) decode(w http.ResponseWriter, r *http.Request, target any, what string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		log := logger.New(r.Context(), "handlers.finance")
		log.Debug().Err(err).Msgf("decode %s body", what)
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

var financeErrMap = newErrMap("handlers.finance",
	E(domain.ErrNotFound, http.StatusNotFound, "not found"),
	E(domain.ErrNotDraft, http.StatusConflict, "expense is already posted"),
	E(domain.ErrCategoryExists, http.StatusConflict, "category already exists"),
	EMsg(domain.ErrUnknownCategory, http.StatusBadRequest),
	EMsg(domain.ErrInvalidKind, http.StatusBadRequest),
	EMsg(domain.ErrInvalidAmount, http.StatusBadRequest),
	EMsg(domain.ErrInvalidName, http.StatusBadRequest),
	EMsg(domain.ErrInvalidPaymentMethod, http.StatusBadRequest),
	EMsg(domain.ErrInvalidDayOfMonth, http.StatusBadRequest),
	EMsg(domain.ErrInvalidPercent, http.StatusBadRequest),
)

func (h *FinanceHandler) writeError(ctx context.Context, w http.ResponseWriter, op string, err error) {
	financeErrMap.Handle(ctx, w, op, err)
}

func ruleFromRequest(body oapigen.CreateExpenseRuleRequest) domain.Rule {
	rule := domain.Rule{
		Name:          body.Name,
		CategoryCode:  body.CategoryCode,
		Vendor:        strOf(body.Vendor),
		PaymentMethod: paymentMethodOf(body.PaymentMethod),
		Amount:        body.Amount,
		DayOfMonth:    body.DayOfMonth,
		IsActive:      true,
	}
	if body.AutoPost != nil {
		rule.AutoPost = *body.AutoPost
	}
	if body.ActiveFrom != nil {
		rule.ActiveFrom = body.ActiveFrom.Time
	}
	if body.ActiveTo != nil {
		to := body.ActiveTo.Time
		rule.ActiveTo = &to
	}
	if body.IsActive != nil {
		rule.IsActive = *body.IsActive
	}
	return rule
}

func toAPIExpense(e domain.Expense) oapigen.Expense {
	out := oapigen.Expense{
		Id:            e.ID,
		SpentAt:       openapi_types.Date{Time: e.SpentAt},
		Amount:        e.Amount,
		CategoryCode:  e.CategoryCode,
		CategoryLabel: e.CategoryLabel,
		PaymentMethod: oapigen.ExpensePaymentMethod(e.PaymentMethod),
		Vendor:        e.Vendor,
		Description:   e.Description,
		Status:        oapigen.ExpenseStatus(e.Status),
		Origin:        oapigen.ExpenseOrigin(e.Origin),
		CreatedAt:     e.CreatedAt,
	}
	if e.CreatedBy != "" {
		out.CreatedBy = &e.CreatedBy
	}
	if e.RuleID != "" {
		out.RuleId = &e.RuleID
	}
	return out
}

func toAPIExpenseRule(r domain.Rule) oapigen.ExpenseRule {
	out := oapigen.ExpenseRule{
		Id:            r.ID,
		Name:          r.Name,
		CategoryCode:  r.CategoryCode,
		CategoryLabel: r.CategoryLabel,
		Vendor:        r.Vendor,
		PaymentMethod: oapigen.ExpensePaymentMethod(r.PaymentMethod),
		Amount:        r.Amount,
		DayOfMonth:    r.DayOfMonth,
		AutoPost:      r.AutoPost,
		ActiveFrom:    openapi_types.Date{Time: r.ActiveFrom},
		IsActive:      r.IsActive,
	}
	if r.ActiveTo != nil {
		out.ActiveTo = &openapi_types.Date{Time: *r.ActiveTo}
	}
	return out
}

func toAPIFinanceMonth(m domain.MonthReport) oapigen.FinanceMonth {
	byCategory := make(map[string]float64, len(m.ExpenseByCategory))
	for code, amount := range m.ExpenseByCategory {
		byCategory[code] = amount
	}
	byKind := make(map[string]float64, len(m.ExpenseByKind))
	for kind, amount := range m.ExpenseByKind {
		byKind[string(kind)] = amount
	}
	return oapigen.FinanceMonth{
		Month:             m.Month,
		IncomeConsult:     m.IncomeConsult,
		IncomeCases:       m.IncomeCases,
		IncomeOther:       m.IncomeOther,
		IncomeTotal:       m.IncomeTotal,
		ExpenseByCategory: byCategory,
		ExpenseByKind:     byKind,
		ExpenseTotal:      m.ExpenseTotal,
		Balance:           m.Balance,
		Cumulative:        m.Cumulative,
		MarketingSpend:    m.MarketingSpend,
		DirectCost:        m.DirectCost,
		GrossProfit:       m.GrossProfit,
		Leads:             m.Leads,
		NewClients:        m.NewClients,
		CohortPayers:      m.CohortPayers,
		PayingClients:     m.PayingClients,
		ConsultCount:      m.ConsultCount,
		CasePaymentCount:  m.CasePaymentCount,
		Cac:               m.CAC,
		Cpl:               m.CPL,
		Romi:              m.ROMI,
		AvgConsultTicket:  m.AvgConsultTicket,
		AvgCaseTicket:     m.AvgCaseTicket,
		MarginPercent:     m.MarginPercent,
		MarketingShare:    m.MarketingShare,
		RevenuePerClient:  m.RevenuePerClient,
		Ltv:               m.LTV,
		LtvToCac:          m.LtvToCac,
		LeadToConsult:     m.LeadToConsult,
		BreakEvenConsults: m.BreakEvenConsults,
		IncomeGrowth:      m.IncomeGrowth,
	}
}

func paymentMethodOf(m *oapigen.ExpensePaymentMethod) domain.PaymentMethod {
	if m == nil {
		return ""
	}
	return domain.PaymentMethod(*m)
}

func strOf(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
