package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/finance"
)

// Month boundaries for timestamptz columns are pinned to the firm's own
// timezone: date_trunc over a timestamptz otherwise follows the DB session
// TimeZone, and the same row would land in different months on two deployments.
const businessTZ = "Europe/Kyiv"

type FinanceRepository struct {
	db *pgxpool.Pool
}

func NewFinanceRepository(db *pgxpool.Pool) *FinanceRepository {
	return &FinanceRepository{db: db}
}

// --- categories ---

func (r *FinanceRepository) ListCategories(ctx context.Context, activeOnly bool) ([]finance.Category, error) {
	const q = `SELECT code, label, kind, is_active, sort_order FROM expense_categories
		WHERE (NOT @active_only::boolean OR is_active)
		ORDER BY sort_order, code`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"active_only": activeOnly})
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var out []finance.Category
	for rows.Next() {
		var c finance.Category
		if err := rows.Scan(&c.Code, &c.Label, &c.Kind, &c.IsActive, &c.SortOrder); err != nil {
			return nil, fmt.Errorf("list categories: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return out, nil
}

func (r *FinanceRepository) CreateCategory(ctx context.Context, c finance.Category) error {
	const q = `INSERT INTO expense_categories (code, label, kind, is_active, sort_order)
		VALUES (@code, @label, @kind, @is_active, @sort_order)`

	_, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"code":       c.Code,
		"label":      c.Label,
		"kind":       string(c.Kind),
		"is_active":  c.IsActive,
		"sort_order": c.SortOrder,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("create category %q: %w", c.Code, finance.ErrCategoryExists)
		}
		return fmt.Errorf("create category %q: %w", c.Code, err)
	}
	return nil
}

func (r *FinanceRepository) UpdateCategory(ctx context.Context, c finance.Category) error {
	const q = `UPDATE expense_categories
		SET label = @label, kind = @kind, is_active = @is_active, sort_order = @sort_order
		WHERE code = @code`

	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"code":       c.Code,
		"label":      c.Label,
		"kind":       string(c.Kind),
		"is_active":  c.IsActive,
		"sort_order": c.SortOrder,
	})
	if err != nil {
		return fmt.Errorf("update category %q: %w", c.Code, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update category %q: %w", c.Code, finance.ErrNotFound)
	}
	return nil
}

// --- expenses ---

const expenseSelect = `SELECT e.id, e.spent_at, e.amount, e.category_code, coalesce(c.label, ''),
		e.payment_method, e.vendor, e.description, e.status, e.origin,
		coalesce(e.rule_id::text, ''), coalesce(e.external_ref, ''), e.created_by, e.created_at
	FROM expenses e
	LEFT JOIN expense_categories c ON c.code = e.category_code`

// One static predicate set instead of a hand-built WHERE: a nil date or an
// empty enum/search short-circuits its own clause, so every filter shape
// runs the same statement.
const expenseFilterWhere = `
	WHERE (@from::date IS NULL OR e.spent_at >= @from::date)
		AND (@to::date IS NULL OR e.spent_at <= @to::date)
		AND (@category_code::text = '' OR e.category_code = @category_code::text)
		AND (@status::text = '' OR e.status = @status::text)
		AND (@origin::text = '' OR e.origin = @origin::text)
		AND (@payment_method::text = '' OR e.payment_method = @payment_method::text)
		AND (@search::text = ''
			OR e.vendor ILIKE '%' || @search::text || '%' ESCAPE '\'
			OR e.description ILIKE '%' || @search::text || '%' ESCAPE '\')`

const insertExpense = `INSERT INTO expenses
		(spent_at, amount, category_code, payment_method, vendor, description, status, origin, rule_id, external_ref, created_by)
	VALUES (@spent_at, @amount, @category_code, @payment_method, @vendor, @description, @status, @origin, @rule_id, @external_ref, @created_by)`

// Drafts stay in the list (staff confirms them from a separate block) and are
// counted by Total, but Sum covers POSTED rows only: it is read as "spent this
// month", and unconfirmed or voided money must not appear in a spend total.
// Both cover the whole filtered set, not the returned page.
func (r *FinanceRepository) ListExpenses(ctx context.Context, filter finance.ExpenseFilter) (finance.ExpenseList, error) {
	args := pgx.NamedArgs{
		"from":           nullTime(filter.From),
		"to":             nullTime(filter.To),
		"category_code":  filter.CategoryCode,
		"status":         string(filter.Status),
		"origin":         string(filter.Origin),
		"payment_method": string(filter.PaymentMethod),
		"search":         escapeLike(filter.Search),
		"limit":          filter.Limit,
		"offset":         filter.Offset,
	}

	var list finance.ExpenseList
	totalQ := `SELECT count(*), coalesce(sum(e.amount) FILTER (WHERE e.status = 'posted'), 0) FROM expenses e` + expenseFilterWhere
	if err := r.db.QueryRow(ctx, totalQ, args).Scan(&list.Total, &list.Sum); err != nil {
		return finance.ExpenseList{}, fmt.Errorf("list expenses: totals: %w", err)
	}

	// nullif(limit, 0) — LIMIT NULL is "no limit", so a zero Limit reads everything.
	pageQ := expenseSelect + expenseFilterWhere + `
		ORDER BY e.spent_at DESC, e.created_at DESC
		LIMIT nullif(@limit::int, 0) OFFSET @offset::int`
	rows, err := r.db.Query(ctx, pageQ, args)
	if err != nil {
		return finance.ExpenseList{}, fmt.Errorf("list expenses: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e finance.Expense
		if err := scanExpense(rows, &e); err != nil {
			return finance.ExpenseList{}, fmt.Errorf("list expenses: scan: %w", err)
		}
		list.Items = append(list.Items, e)
	}
	if err := rows.Err(); err != nil {
		return finance.ExpenseList{}, fmt.Errorf("list expenses: %w", err)
	}
	return list, nil
}

func (r *FinanceRepository) GetExpense(ctx context.Context, id string) (finance.Expense, error) {
	q := expenseSelect + ` WHERE e.id = @id`
	var e finance.Expense
	err := scanExpense(r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": id}), &e)
	if errors.Is(err, pgx.ErrNoRows) {
		return finance.Expense{}, fmt.Errorf("get expense %q: %w", id, finance.ErrNotFound)
	}
	if err != nil {
		return finance.Expense{}, fmt.Errorf("get expense %q: %w", id, err)
	}
	return e, nil
}

func (r *FinanceRepository) CreateExpense(ctx context.Context, e finance.Expense) (finance.Expense, error) {
	e = expenseWithDefaults(e)
	q := insertExpense + `
		RETURNING id, created_at, coalesce((SELECT label FROM expense_categories WHERE code = expenses.category_code), '')`

	err := r.db.QueryRow(ctx, q, expenseArgs(e)).Scan(&e.ID, &e.CreatedAt, &e.CategoryLabel)
	if err != nil {
		return finance.Expense{}, fmt.Errorf("create expense: %w", translateExpenseWrite(err))
	}
	return e, nil
}

// Status/origin/rule_id/external_ref are provenance, not editable fields:
// rewriting external_ref would break the generator's idempotency, and a
// draft is promoted through ConfirmExpense.
func (r *FinanceRepository) UpdateExpense(ctx context.Context, e finance.Expense) error {
	const q = `UPDATE expenses SET
			spent_at = @spent_at,
			amount = @amount,
			category_code = @category_code,
			payment_method = @payment_method,
			vendor = @vendor,
			description = @description
		WHERE id = @id`

	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"id":             e.ID,
		"spent_at":       e.SpentAt,
		"amount":         e.Amount,
		"category_code":  e.CategoryCode,
		"payment_method": string(e.PaymentMethod),
		"vendor":         e.Vendor,
		"description":    e.Description,
	})
	if err != nil {
		return fmt.Errorf("update expense %q: %w", e.ID, translateExpenseWrite(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update expense %q: %w", e.ID, finance.ErrNotFound)
	}
	return nil
}

func (r *FinanceRepository) DeleteExpense(ctx context.Context, id string) error {
	const q = `DELETE FROM expenses WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id})
	if err != nil {
		return fmt.Errorf("delete expense %q: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete expense %q: %w", id, finance.ErrNotFound)
	}
	return nil
}

func (r *FinanceRepository) ConfirmExpense(ctx context.Context, id string) error {
	const q = `UPDATE expenses SET status = 'posted' WHERE id = @id AND status = 'draft'`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id})
	if err != nil {
		return fmt.Errorf("confirm expense %q: %w", id, err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	// Zero rows is ambiguous — no such expense, or one that is already
	// posted. The caller needs to tell those apart.
	const statusQ = `SELECT status FROM expenses WHERE id = @id`
	var status finance.Status
	err = r.db.QueryRow(ctx, statusQ, pgx.NamedArgs{"id": id}).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("confirm expense %q: %w", id, finance.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("confirm expense %q: %w", id, err)
	}
	return fmt.Errorf("confirm expense %q: %w", id, finance.ErrNotDraft)
}

// VoidExpense keeps the row (and its external_ref) so the generator will not
// re-create what staff retired; every P&L aggregate filters on 'posted'.
func (r *FinanceRepository) VoidExpense(ctx context.Context, id string) error {
	const q = `UPDATE expenses SET status = 'void' WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id})
	if err != nil {
		return fmt.Errorf("void expense %q: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("void expense %q: %w", id, finance.ErrNotFound)
	}
	return nil
}

// The unique index on external_ref is partial, so the conflict target has to
// repeat its predicate for inference. A re-run of the generator hits the
// index and inserts nothing — that is a no-op, not an error.
func (r *FinanceRepository) InsertGenerated(ctx context.Context, e finance.Expense) (bool, error) {
	e = expenseWithDefaults(e)
	q := insertExpense + `
		ON CONFLICT (external_ref) WHERE external_ref IS NOT NULL DO NOTHING`

	tag, err := r.db.Exec(ctx, q, expenseArgs(e))
	if err != nil {
		return false, fmt.Errorf("insert generated expense %q: %w", e.ExternalRef, translateExpenseWrite(err))
	}
	return tag.RowsAffected() > 0, nil
}

// --- rules ---

const ruleSelect = `SELECT r.id, r.name, r.category_code, coalesce(c.label, ''), r.vendor, r.payment_method,
		r.amount, r.day_of_month, r.auto_post, r.active_from, r.active_to, r.is_active, r.created_by, r.created_at
	FROM expense_rules r
	LEFT JOIN expense_categories c ON c.code = r.category_code`

func (r *FinanceRepository) ListRules(ctx context.Context, activeOnly bool) ([]finance.Rule, error) {
	q := ruleSelect + `
		WHERE (NOT @active_only::boolean OR r.is_active)
		ORDER BY r.day_of_month, r.name`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"active_only": activeOnly})
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()

	var out []finance.Rule
	for rows.Next() {
		var rule finance.Rule
		if err := scanRule(rows, &rule); err != nil {
			return nil, fmt.Errorf("list rules: scan: %w", err)
		}
		out = append(out, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	return out, nil
}

func (r *FinanceRepository) CreateRule(ctx context.Context, rule finance.Rule) (finance.Rule, error) {
	const q = `INSERT INTO expense_rules
			(name, category_code, vendor, payment_method, amount, day_of_month, auto_post, active_from, active_to, is_active, created_by)
		VALUES (@name, @category_code, @vendor, @payment_method, @amount, @day_of_month, @auto_post, @active_from, @active_to, @is_active, @created_by)
		RETURNING id, created_at, coalesce((SELECT label FROM expense_categories WHERE code = expense_rules.category_code), '')`

	if rule.PaymentMethod == "" {
		rule.PaymentMethod = finance.PaymentCard
	}
	err := r.db.QueryRow(ctx, q, ruleArgs(rule)).Scan(&rule.ID, &rule.CreatedAt, &rule.CategoryLabel)
	if err != nil {
		return finance.Rule{}, fmt.Errorf("create rule %q: %w", rule.Name, translateExpenseWrite(err))
	}
	return rule, nil
}

func (r *FinanceRepository) UpdateRule(ctx context.Context, rule finance.Rule) error {
	const q = `UPDATE expense_rules SET
			name = @name,
			category_code = @category_code,
			vendor = @vendor,
			payment_method = @payment_method,
			amount = @amount,
			day_of_month = @day_of_month,
			auto_post = @auto_post,
			active_from = @active_from,
			active_to = @active_to,
			is_active = @is_active
		WHERE id = @id`

	args := ruleArgs(rule)
	args["id"] = rule.ID
	tag, err := r.db.Exec(ctx, q, args)
	if err != nil {
		return fmt.Errorf("update rule %q: %w", rule.ID, translateExpenseWrite(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update rule %q: %w", rule.ID, finance.ErrNotFound)
	}
	return nil
}

func (r *FinanceRepository) DeleteRule(ctx context.Context, id string) error {
	const q = `DELETE FROM expense_rules WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id})
	if err != nil {
		return fmt.Errorf("delete rule %q: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete rule %q: %w", id, finance.ErrNotFound)
	}
	return nil
}

// --- other income ---

func (r *FinanceRepository) ListOtherIncome(ctx context.Context, from, to time.Time) ([]finance.OtherIncome, error) {
	const q = `SELECT id, received_at, amount, source, description, coalesce(external_ref, ''), created_by, created_at
		FROM other_income
		WHERE (@from::date IS NULL OR received_at >= @from::date)
			AND (@to::date IS NULL OR received_at <= @to::date)
		ORDER BY received_at DESC, created_at DESC`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"from": nullTime(from), "to": nullTime(to)})
	if err != nil {
		return nil, fmt.Errorf("list other income: %w", err)
	}
	defer rows.Close()

	var out []finance.OtherIncome
	for rows.Next() {
		var i finance.OtherIncome
		if err := rows.Scan(&i.ID, &i.ReceivedAt, &i.Amount, &i.Source, &i.Description, &i.ExternalRef, &i.CreatedBy, &i.CreatedAt); err != nil {
			return nil, fmt.Errorf("list other income: scan: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list other income: %w", err)
	}
	return out, nil
}

func (r *FinanceRepository) CreateOtherIncome(ctx context.Context, i finance.OtherIncome) (finance.OtherIncome, error) {
	const q = `INSERT INTO other_income (received_at, amount, source, description, created_by)
		VALUES (@received_at, @amount, @source, @description, @created_by)
		RETURNING id, created_at`

	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"received_at": i.ReceivedAt,
		"amount":      i.Amount,
		"source":      i.Source,
		"description": i.Description,
		"created_by":  i.CreatedBy,
	}).Scan(&i.ID, &i.CreatedAt)
	if err != nil {
		return finance.OtherIncome{}, fmt.Errorf("create other income: %w", err)
	}
	return i, nil
}

// InsertOtherIncomeGenerated backs the reconciliation top-up. Same shape as
// InsertGenerated for expenses, and the same reason: the partial unique index
// needs its predicate repeated for ON CONFLICT to infer it, and a repeat run
// must be a no-op rather than a second booking of the same missing month.
func (r *FinanceRepository) InsertOtherIncomeGenerated(ctx context.Context, i finance.OtherIncome) (bool, error) {
	const q = `INSERT INTO other_income (received_at, amount, source, description, external_ref, created_by)
		VALUES (@received_at, @amount, @source, @description, @external_ref, @created_by)
		ON CONFLICT (external_ref) WHERE external_ref IS NOT NULL DO NOTHING`

	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"received_at":  i.ReceivedAt,
		"amount":       i.Amount,
		"source":       i.Source,
		"description":  i.Description,
		"external_ref": nullText(i.ExternalRef),
		"created_by":   i.CreatedBy,
	})
	if err != nil {
		return false, fmt.Errorf("insert other income %q: %w", i.ExternalRef, err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *FinanceRepository) DeleteOtherIncome(ctx context.Context, id string) error {
	const q = `DELETE FROM other_income WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id})
	if err != nil {
		return fmt.Errorf("delete other income %q: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete other income %q: %w", id, finance.ErrNotFound)
	}
	return nil
}

// --- report source ---

// MonthlyFacts drives the whole report off generate_series over months, so a
// month with no activity at all still comes back as a zero row and never
// shifts the report's columns.
//
// Consultation revenue is bucketed by scheduled_at — the month the
// consultation actually happened — while leadstats buckets its own numbers by
// created_at ("when it was booked"). The price>0 AND status='completed' filter
// is the same one, so both pages agree on what "earned" means, but NOT on
// which month a consultation booked in one month and held in the next belongs
// to. That difference is deliberate here: a P&L is about when the money moved.
//
// scheduled_at is a timestamptz, and date_trunc over one follows the DB session
// TimeZone — so the month is pinned to businessTZ instead, or the same row
// would land in different months on two deployments.
//
// Only posted expenses are summed: a draft is a generated suggestion nobody
// has confirmed, and counting it would overstate spend.
func (r *FinanceRepository) MonthlyFacts(ctx context.Context, from, to time.Time) ([]finance.MonthFacts, error) {
	const factsQ = `
		WITH bounds AS (
			SELECT date_trunc('month', @from::date)::date AS lo,
				(date_trunc('month', @to::date) + interval '1 month')::date AS hi
		),
		months AS (
			SELECT generate_series(b.lo::timestamp, (b.hi - interval '1 month')::timestamp, interval '1 month')::date AS month
			FROM bounds b
		),
		consult AS (
			SELECT date_trunc('month', c.scheduled_at AT TIME ZONE '` + businessTZ + `')::date AS month,
				coalesce(sum(c.price) FILTER (WHERE c.price > 0 AND c.status = 'completed'), 0) AS revenue,
				count(*) FILTER (WHERE c.price > 0 AND c.status = 'completed') AS held
			FROM consultations c, bounds b
			WHERE c.scheduled_at >= b.lo AND c.scheduled_at < b.hi
			GROUP BY 1
		),
		case_paid AS (
			SELECT date_trunc('month', p.paid_at)::date AS month, sum(p.amount) AS paid, count(*) AS payments
			FROM case_payments p, bounds b
			WHERE p.paid_at >= b.lo AND p.paid_at < b.hi
			GROUP BY 1
		),
		other_inc AS (
			SELECT date_trunc('month', o.received_at)::date AS month, sum(o.amount) AS amount
			FROM other_income o, bounds b
			WHERE o.received_at >= b.lo AND o.received_at < b.hi
			GROUP BY 1
		),
		lead_count AS (
			SELECT date_trunc('month', l.received_at AT TIME ZONE '` + businessTZ + `')::date AS month, count(*) AS leads
			FROM leads l, bounds b
			WHERE l.received_at >= b.lo AND l.received_at < b.hi
			GROUP BY 1
		),
		first_lead AS (
			SELECT client_id, min(received_at) AS first_lead_at
			FROM leads
			WHERE client_id IS NOT NULL
			GROUP BY client_id
		),
		new_clients AS (
			SELECT date_trunc('month', f.first_lead_at AT TIME ZONE '` + businessTZ + `')::date AS month, count(*) AS clients
			FROM first_lead f, bounds b
			WHERE f.first_lead_at >= b.lo AND f.first_lead_at < b.hi
			GROUP BY 1
		),
		-- Everything a client has EVER paid, with no date bound: acquisition cost
		-- belongs to the month they arrived, but the revenue proving it out can
		-- land any time later. This is what keeps LTV from collapsing into ROMI.
		client_lifetime AS (
			SELECT cl.id AS client_id,
				coalesce((SELECT sum(c.price) FROM consultations c
					WHERE c.client_id = cl.id AND c.price > 0 AND c.status = 'completed'), 0)
				+ coalesce((SELECT sum(p.amount) FROM case_payments p
					JOIN cases k ON k.id = p.case_id WHERE k.client_id = cl.id), 0) AS paid
			FROM clients cl
		),
		cohort AS (
			SELECT date_trunc('month', f.first_lead_at AT TIME ZONE '` + businessTZ + `')::date AS month,
				count(*) FILTER (WHERE lt.paid > 0) AS payers,
				coalesce(sum(lt.paid), 0) AS revenue
			FROM first_lead f
			JOIN client_lifetime lt ON lt.client_id = f.client_id, bounds b
			WHERE f.first_lead_at >= b.lo AND f.first_lead_at < b.hi
			GROUP BY 1
		),
		-- distinct clients who paid anything IN the month, for revenue per client
		paying AS (
			SELECT month, count(DISTINCT client_id) AS clients FROM (
				SELECT date_trunc('month', c.scheduled_at AT TIME ZONE '` + businessTZ + `')::date AS month, c.client_id
				FROM consultations c, bounds b
				WHERE c.price > 0 AND c.status = 'completed'
					AND c.scheduled_at >= b.lo AND c.scheduled_at < b.hi
				UNION ALL
				SELECT date_trunc('month', p.paid_at)::date AS month, k.client_id
				FROM case_payments p JOIN cases k ON k.id = p.case_id, bounds b
				WHERE p.paid_at >= b.lo AND p.paid_at < b.hi
			) paid_rows
			GROUP BY 1
		)
		SELECT m.month,
			coalesce(cr.revenue, 0),
			coalesce(cr.held, 0),
			coalesce(cp.paid, 0),
			coalesce(cp.payments, 0),
			coalesce(oi.amount, 0),
			coalesce(lc.leads, 0),
			coalesce(nc.clients, 0),
			coalesce(co.payers, 0),
			coalesce(co.revenue, 0),
			coalesce(pa.clients, 0)
		FROM months m
		LEFT JOIN consult cr ON cr.month = m.month
		LEFT JOIN case_paid cp ON cp.month = m.month
		LEFT JOIN other_inc oi ON oi.month = m.month
		LEFT JOIN lead_count lc ON lc.month = m.month
		LEFT JOIN new_clients nc ON nc.month = m.month
		LEFT JOIN cohort co ON co.month = m.month
		LEFT JOIN paying pa ON pa.month = m.month
		ORDER BY m.month`

	args := pgx.NamedArgs{"from": from, "to": to}
	rows, err := r.db.Query(ctx, factsQ, args)
	if err != nil {
		return nil, fmt.Errorf("monthly facts: %w", err)
	}
	defer rows.Close()

	var out []finance.MonthFacts
	byMonth := map[string]int{}
	for rows.Next() {
		var monthAt time.Time
		var f finance.MonthFacts
		err := rows.Scan(
			&monthAt,
			&f.ConsultRevenue,
			&f.ConsultCount,
			&f.CasePaid,
			&f.CasePaymentCount,
			&f.OtherIncome,
			&f.Leads,
			&f.NewClients,
			&f.CohortPayers,
			&f.CohortRevenue,
			&f.PayingClients,
		)
		if err != nil {
			return nil, fmt.Errorf("monthly facts: scan: %w", err)
		}
		f.Month = finance.MonthKey(monthAt)
		f.ExpenseByCategory = map[string]float64{}
		byMonth[f.Month] = len(out)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("monthly facts: %w", err)
	}

	const spendQ = `
		SELECT date_trunc('month', spent_at)::date, category_code, sum(amount)
		FROM expenses
		WHERE status = 'posted'
			AND spent_at >= date_trunc('month', @from::date)
			AND spent_at < date_trunc('month', @to::date) + interval '1 month'
		GROUP BY 1, 2`

	spendRows, err := r.db.Query(ctx, spendQ, args)
	if err != nil {
		return nil, fmt.Errorf("monthly facts: expenses: %w", err)
	}
	defer spendRows.Close()

	for spendRows.Next() {
		var monthAt time.Time
		var code string
		var amount float64
		if err := spendRows.Scan(&monthAt, &code, &amount); err != nil {
			return nil, fmt.Errorf("monthly facts: expenses: scan: %w", err)
		}
		if idx, ok := byMonth[finance.MonthKey(monthAt)]; ok {
			out[idx].ExpenseByCategory[code] = amount
		}
	}
	if err := spendRows.Err(); err != nil {
		return nil, fmt.Errorf("monthly facts: expenses: %w", err)
	}
	return out, nil
}

// Same income/expense definitions as MonthlyFacts, with no lower bound and
// drafts excluded — everything strictly before from, as one number.
func (r *FinanceRepository) BalanceBefore(ctx context.Context, from time.Time) (float64, error) {
	const q = `
		SELECT
			coalesce((SELECT sum(price) FROM consultations
				WHERE price > 0 AND status = 'completed'
					AND (scheduled_at AT TIME ZONE '` + businessTZ + `') < @from::date), 0)
			+ coalesce((SELECT sum(amount) FROM case_payments WHERE paid_at < @from::date), 0)
			+ coalesce((SELECT sum(amount) FROM other_income WHERE received_at < @from::date), 0)
			- coalesce((SELECT sum(amount) FROM expenses
				WHERE status = 'posted' AND spent_at < @from::date), 0)`

	var balance float64
	if err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"from": from}).Scan(&balance); err != nil {
		return 0, fmt.Errorf("balance before: %w", err)
	}
	return balance, nil
}

// DataRange bounds what the finance page can show. The money span is what "all
// time" uses; leads are reported separately because they keep arriving long
// after the last recorded payment, and defaulting to them fills the report with
// empty columns. least/greatest skip NULLs, so an empty table drops out of the
// comparison instead of nulling the answer.
func (r *FinanceRepository) DataRange(ctx context.Context) (time.Time, time.Time, time.Time, error) {
	const q = `
		SELECT
			least(
				(SELECT min(spent_at) FROM expenses WHERE status = 'posted'),
				(SELECT min((scheduled_at AT TIME ZONE '` + businessTZ + `')::date) FROM consultations WHERE price > 0 AND status = 'completed'),
				(SELECT min(paid_at) FROM case_payments),
				(SELECT min(received_at) FROM other_income)
			),
			greatest(
				(SELECT max(spent_at) FROM expenses WHERE status = 'posted'),
				(SELECT max((scheduled_at AT TIME ZONE '` + businessTZ + `')::date) FROM consultations WHERE price > 0 AND status = 'completed'),
				(SELECT max(paid_at) FROM case_payments),
				(SELECT max(received_at) FROM other_income)
			),
			(SELECT max((received_at AT TIME ZONE '` + businessTZ + `')::date) FROM leads)`

	var firstMoney, lastMoney, lastActivity *time.Time
	if err := r.db.QueryRow(ctx, q).Scan(&firstMoney, &lastMoney, &lastActivity); err != nil {
		return time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("data range: %w", err)
	}
	if firstMoney == nil || lastMoney == nil {
		return time.Time{}, time.Time{}, time.Time{}, nil
	}
	activity := *lastMoney
	if lastActivity != nil && lastActivity.After(activity) {
		activity = *lastActivity
	}
	return *firstMoney, *lastMoney, activity, nil
}

// Receivable is what clients still owe across every case, ignoring the report
// window on purpose: money owed on a case opened last year is owed today, and a
// short window must not make it disappear from the number. Same definition as
// leadstats' CaseOwed, so the two pages can't disagree about the debt.
func (r *FinanceRepository) Receivable(ctx context.Context) (float64, error) {
	// A cancelled case's unpaid fee is not owed to anyone, and counting it made
	// the debt figure permanently overstated.
	const q = `SELECT coalesce(sum(greatest(fee - paid_amount, 0)), 0) FROM cases WHERE status <> 'cancelled'`

	var owed float64
	if err := r.db.QueryRow(ctx, q).Scan(&owed); err != nil {
		return 0, fmt.Errorf("receivable: %w", err)
	}
	return owed, nil
}

// Attribution prefers cases.advocate_id and falls back to an exact
// advocate_name match, which is how the pre-roster history (surname only) still
// reaches the person it belongs to. A name that matches nobody stays out.
func (r *FinanceRepository) AdvocateCollections(ctx context.Context, from, to time.Time) ([]finance.AdvocateCollection, error) {
	const q = `
		SELECT a.id::text, a.full_name, sum(p.amount), a.commission_percent
		FROM cases c
		-- Cases opened before the advocate roster carry only a free-text surname.
		-- coalesce prefers the real link and falls back to an EXACT name match —
		-- deterministic, and never fuzzy: attributing someone's payout by
		-- approximate name is not a mistake worth risking.
		JOIN advocates a ON a.id = coalesce(
			c.advocate_id,
			(SELECT x.id FROM advocates x WHERE x.full_name = c.advocate_name LIMIT 1)
		)
		JOIN case_payments p ON p.case_id = c.id
		WHERE p.paid_at >= @from::date AND p.paid_at <= @to::date
		GROUP BY a.id, a.full_name, a.commission_percent
		ORDER BY 3 DESC`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"from": from, "to": to})
	if err != nil {
		return nil, fmt.Errorf("advocate collections: %w", err)
	}
	defer rows.Close()

	var out []finance.AdvocateCollection
	for rows.Next() {
		var c finance.AdvocateCollection
		if err := rows.Scan(&c.AdvocateID, &c.AdvocateName, &c.Collected, &c.CommissionPercent); err != nil {
			return nil, fmt.Errorf("advocate collections: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("advocate collections: %w", err)
	}
	return out, nil
}

// AdvocatePayouts splits what left the advocates category into "we know who this
// went to" and "we do not". Generated payouts carry the advocate in their
// external ref ("advocate:<id>:<month>"); the lump sums imported from the
// spreadsheet name nobody, and spreading them across advocates by guesswork
// would invent a number.
func (r *FinanceRepository) AdvocatePayouts(ctx context.Context, from, to time.Time) (map[string]float64, float64, error) {
	const q = `
		SELECT
			CASE WHEN external_ref LIKE 'advocate:%' THEN split_part(external_ref, ':', 2) ELSE '' END AS advocate_id,
			sum(amount)
		FROM expenses
		WHERE status = 'posted'
			AND category_code = 'advocates'
			AND spent_at >= @from::date AND spent_at <= @to::date
		GROUP BY 1`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"from": from, "to": to})
	if err != nil {
		return nil, 0, fmt.Errorf("advocate payouts: %w", err)
	}
	defer rows.Close()

	attributed := map[string]float64{}
	var unattributed float64
	for rows.Next() {
		var id string
		var amount float64
		if err := rows.Scan(&id, &amount); err != nil {
			return nil, 0, fmt.Errorf("advocate payouts: scan: %w", err)
		}
		if id == "" {
			unattributed += amount
			continue
		}
		attributed[id] = amount
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("advocate payouts: %w", err)
	}
	return attributed, unattributed, nil
}

// --- helpers ---

func scanExpense(row pgx.Row, e *finance.Expense) error {
	return row.Scan(
		&e.ID, &e.SpentAt, &e.Amount, &e.CategoryCode, &e.CategoryLabel,
		&e.PaymentMethod, &e.Vendor, &e.Description, &e.Status, &e.Origin,
		&e.RuleID, &e.ExternalRef, &e.CreatedBy, &e.CreatedAt,
	)
}

func scanRule(row pgx.Row, rule *finance.Rule) error {
	var activeTo *time.Time
	err := row.Scan(
		&rule.ID, &rule.Name, &rule.CategoryCode, &rule.CategoryLabel, &rule.Vendor, &rule.PaymentMethod,
		&rule.Amount, &rule.DayOfMonth, &rule.AutoPost, &rule.ActiveFrom, &activeTo,
		&rule.IsActive, &rule.CreatedBy, &rule.CreatedAt,
	)
	if err != nil {
		return err
	}
	rule.ActiveTo = activeTo
	return nil
}

func expenseWithDefaults(e finance.Expense) finance.Expense {
	if e.Status == "" {
		e.Status = finance.StatusPosted
	}
	if e.Origin == "" {
		e.Origin = finance.OriginManual
	}
	if e.PaymentMethod == "" {
		e.PaymentMethod = finance.PaymentCard
	}
	return e
}

func expenseArgs(e finance.Expense) pgx.NamedArgs {
	return pgx.NamedArgs{
		"spent_at":       e.SpentAt,
		"amount":         e.Amount,
		"category_code":  e.CategoryCode,
		"payment_method": string(e.PaymentMethod),
		"vendor":         e.Vendor,
		"description":    e.Description,
		"status":         string(e.Status),
		"origin":         string(e.Origin),
		"rule_id":        nullText(e.RuleID),
		"external_ref":   nullText(e.ExternalRef),
		"created_by":     e.CreatedBy,
	}
}

func ruleArgs(rule finance.Rule) pgx.NamedArgs {
	return pgx.NamedArgs{
		"name":           rule.Name,
		"category_code":  rule.CategoryCode,
		"vendor":         rule.Vendor,
		"payment_method": string(rule.PaymentMethod),
		"amount":         rule.Amount,
		"day_of_month":   rule.DayOfMonth,
		"auto_post":      rule.AutoPost,
		"active_from":    rule.ActiveFrom,
		"active_to":      rule.ActiveTo,
		"is_active":      rule.IsActive,
		"created_by":     rule.CreatedBy,
	}
}

// ConstraintName disambiguates the FKs sharing SQLSTATE 23503 — only the
// category_code one means the caller named a category that doesn't exist.
func translateExpenseWrite(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	if pgErr.Code == "23503" {
		switch pgErr.ConstraintName {
		case "expenses_category_code_fkey", "expense_rules_category_code_fkey":
			return finance.ErrUnknownCategory
		}
	}
	return err
}

// An empty external_ref must land as NULL: the unique index on it is partial,
// so a stored empty string would collide across unrelated manual rows.
func nullText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// A raw % or _ typed into the search box is a literal, not a LIKE wildcard —
// without this a single "%" matches the entire ledger.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	return strings.ReplaceAll(s, "_", `\_`)
}
