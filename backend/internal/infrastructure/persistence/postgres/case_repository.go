package postgres

import (
	"context"
	"fmt"

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
	return out, nil
}

func (r *CaseRepository) AddPayment(ctx context.Context, caseID string, amount float64) (cases.Case, error) {
	const q = `UPDATE cases SET paid_amount = paid_amount + @amount WHERE id = @id
		RETURNING id, client_id, coalesce(consultation_id::text, ''), coalesce(advocate_id::text, ''), advocate_name, category, fee, paid_amount, status, description, created_by, created_at`
	var c cases.Case
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": caseID, "amount": amount}).Scan(
		&c.ID, &c.ClientID, &c.ConsultationID, &c.AdvocateID, &c.AdvocateName, &c.Category, &c.Fee, &c.PaidAmount, &c.Status, &c.Description, &c.CreatedBy, &c.CreatedAt,
	)
	if err != nil {
		return cases.Case{}, fmt.Errorf("add payment to case %q: %w", caseID, err)
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
