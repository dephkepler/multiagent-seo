package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/cases"
)

type CaseRepository struct {
	db *pgxpool.Pool
}

func NewCaseRepository(db *pgxpool.Pool) *CaseRepository {
	return &CaseRepository{db: db}
}

func (r *CaseRepository) Save(ctx context.Context, c cases.Case) (cases.Case, error) {
	if c.Status == "" {
		c.Status = cases.StatusInProgress
	}
	const q = `INSERT INTO cases (client_id, consultation_id, advocate_id, advocate_name, category, fee, paid_amount, status, description, created_by)
		VALUES (@client_id, @consultation_id, @advocate_id, @advocate_name, @category, @fee, @paid_amount, @status, @description, @created_by)
		RETURNING id, created_at`

	var consultationID *string
	if c.ConsultationID != "" {
		consultationID = &c.ConsultationID
	}
	var advocateID *string
	if c.AdvocateID != "" {
		advocateID = &c.AdvocateID
	}

	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"client_id":       c.ClientID,
		"consultation_id": consultationID,
		"advocate_id":     advocateID,
		"advocate_name":   c.AdvocateName,
		"category":        c.Category,
		"fee":             c.Fee,
		"paid_amount":     c.PaidAmount,
		"status":          c.Status,
		"description":     c.Description,
		"created_by":      c.CreatedBy,
	}).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return cases.Case{}, fmt.Errorf("save case for client %q: %w", c.ClientID, err)
	}
	return c, nil
}

func (r *CaseRepository) Get(ctx context.Context, caseID string) (cases.Case, error) {
	const q = `SELECT id, client_id, coalesce(consultation_id::text, ''), coalesce(advocate_id::text, ''), advocate_name, category, fee, paid_amount, status, description, created_by, created_at
		FROM cases WHERE id = @id`
	var c cases.Case
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": caseID}).Scan(
		&c.ID, &c.ClientID, &c.ConsultationID, &c.AdvocateID, &c.AdvocateName, &c.Category, &c.Fee, &c.PaidAmount, &c.Status, &c.Description, &c.CreatedBy, &c.CreatedAt,
	)
	if err != nil {
		return cases.Case{}, fmt.Errorf("get case %q: %w", caseID, err)
	}
	return c, nil
}

func (r *CaseRepository) ListByClient(ctx context.Context, clientID string) ([]cases.Case, error) {
	const q = `SELECT id, client_id, coalesce(consultation_id::text, ''), coalesce(advocate_id::text, ''), advocate_name, category, fee, paid_amount, status, description, created_by, created_at
		FROM cases WHERE client_id = @client_id
		ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"client_id": clientID})
	if err != nil {
		return nil, fmt.Errorf("list cases for client %q: %w", clientID, err)
	}
	defer rows.Close()

	var out []cases.Case
	for rows.Next() {
		var c cases.Case
		if err := rows.Scan(
			&c.ID, &c.ClientID, &c.ConsultationID, &c.AdvocateID, &c.AdvocateName, &c.Category, &c.Fee, &c.PaidAmount, &c.Status, &c.Description, &c.CreatedBy, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("list cases for client %q: scan: %w", clientID, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cases for client %q: %w", clientID, err)
	}
	if len(out) == 0 {
		return out, nil
	}

	ids := make([]string, len(out))
	byID := make(map[string]*cases.Case, len(out))
	for i := range out {
		ids[i] = out[i].ID
		byID[out[i].ID] = &out[i]
	}
	const paymentsQ = `SELECT case_id, id, amount, paid_at FROM case_payments
		WHERE case_id = ANY(@ids::uuid[]) ORDER BY paid_at, created_at`
	payRows, err := r.db.Query(ctx, paymentsQ, pgx.NamedArgs{"ids": ids})
	if err != nil {
		return nil, fmt.Errorf("list cases for client %q: payments: %w", clientID, err)
	}
	defer payRows.Close()
	for payRows.Next() {
		var caseID string
		var p cases.Payment
		if err := payRows.Scan(&caseID, &p.ID, &p.Amount, &p.PaidAt); err != nil {
			return nil, fmt.Errorf("list cases for client %q: payments: scan: %w", clientID, err)
		}
		if c, ok := byID[caseID]; ok {
			c.Payments = append(c.Payments, p)
		}
	}
	if err := payRows.Err(); err != nil {
		return nil, fmt.Errorf("list cases for client %q: payments: %w", clientID, err)
	}
	return out, nil
}

// AddPayment writes the case_payments ledger row and bumps the running
// paid_amount total together in one transaction — the only place in this
// package that does, deliberately: unlike a single-row write, these two
// are meant to always agree (a ledger entry with no corresponding total
// bump, or vice versa, is a real accounting discrepancy on money the
// business is owed), so a partial failure here is worth guarding against
// even though nothing else in this codebase needs a transaction.
func (r *CaseRepository) AddPayment(ctx context.Context, caseID string, amount float64, paidAt time.Time, createdBy string) (cases.Case, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return cases.Case{}, fmt.Errorf("add payment to case %q: begin: %w", caseID, err)
	}
	defer tx.Rollback(ctx) // no-op once committed

	const insertQ = `INSERT INTO case_payments (case_id, amount, paid_at, created_by)
		VALUES (@case_id, @amount, @paid_at, @created_by)`
	if _, err := tx.Exec(ctx, insertQ, pgx.NamedArgs{
		"case_id": caseID, "amount": amount, "paid_at": paidAt, "created_by": createdBy,
	}); err != nil {
		return cases.Case{}, fmt.Errorf("record payment for case %q: %w", caseID, err)
	}

	const updateQ = `UPDATE cases SET paid_amount = paid_amount + @amount WHERE id = @id
		RETURNING id, client_id, coalesce(consultation_id::text, ''), coalesce(advocate_id::text, ''), advocate_name, category, fee, paid_amount, status, description, created_by, created_at`
	var c cases.Case
	err = tx.QueryRow(ctx, updateQ, pgx.NamedArgs{"id": caseID, "amount": amount}).Scan(
		&c.ID, &c.ClientID, &c.ConsultationID, &c.AdvocateID, &c.AdvocateName, &c.Category, &c.Fee, &c.PaidAmount, &c.Status, &c.Description, &c.CreatedBy, &c.CreatedAt,
	)
	if err != nil {
		return cases.Case{}, fmt.Errorf("add payment to case %q: %w", caseID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return cases.Case{}, fmt.Errorf("add payment to case %q: commit: %w", caseID, err)
	}
	return c, nil
}

func (r *CaseRepository) UpdateStatus(ctx context.Context, caseID, status string) error {
	const q = `UPDATE cases SET status = @status WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"status": status, "id": caseID})
	if err != nil {
		return fmt.Errorf("update case status %q: %w", caseID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update case status: no case with id %q", caseID)
	}
	return nil
}
