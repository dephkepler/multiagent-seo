package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/clientdetail"
)

type ClientDetailRepository struct {
	db *pgxpool.Pool
}

func NewClientDetailRepository(db *pgxpool.Pool) *ClientDetailRepository {
	return &ClientDetailRepository{db: db}
}

// Get reads the client row plus its full history in four separate
// queries — one client card is looked up rarely enough (a staff member
// opening one at a time) that four simple queries beat one join sprawling
// across leads/consultations/cases/client_notes with mismatched row counts.
func (r *ClientDetailRepository) Get(ctx context.Context, clientID string) (clientdetail.Detail, error) {
	var d clientdetail.Detail

	const clientQ = `SELECT id, name, phone, first_seen_at, last_seen_at FROM clients WHERE id = @id`
	err := r.db.QueryRow(ctx, clientQ, pgx.NamedArgs{"id": clientID}).Scan(
		&d.Client.ID, &d.Client.Name, &d.Client.Phone, &d.Client.FirstSeenAt, &d.Client.LastSeenAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return clientdetail.Detail{}, clientdetail.ErrNotFound
	}
	if err != nil {
		return clientdetail.Detail{}, fmt.Errorf("get client %q: %w", clientID, err)
	}

	const leadsQ = `SELECT id, received_at, message, page FROM leads WHERE client_id = @id ORDER BY received_at DESC`
	leadRows, err := r.db.Query(ctx, leadsQ, pgx.NamedArgs{"id": clientID})
	if err != nil {
		return clientdetail.Detail{}, fmt.Errorf("get client %q: leads: %w", clientID, err)
	}
	d.Leads, err = pgx.CollectRows(leadRows, func(row pgx.CollectableRow) (clientdetail.Lead, error) {
		var l clientdetail.Lead
		err := row.Scan(&l.ID, &l.ReceivedAt, &l.Message, &l.Page)
		return l, err
	})
	if err != nil {
		return clientdetail.Detail{}, fmt.Errorf("get client %q: leads: %w", clientID, err)
	}

	const consultQ = `SELECT id, scheduled_at, price, status, case_note FROM consultations
		WHERE client_id = @id ORDER BY scheduled_at DESC`
	consultRows, err := r.db.Query(ctx, consultQ, pgx.NamedArgs{"id": clientID})
	if err != nil {
		return clientdetail.Detail{}, fmt.Errorf("get client %q: consultations: %w", clientID, err)
	}
	d.Consultations, err = pgx.CollectRows(consultRows, func(row pgx.CollectableRow) (clientdetail.Consultation, error) {
		var c clientdetail.Consultation
		err := row.Scan(&c.ID, &c.ScheduledAt, &c.Price, &c.Status, &c.CaseNote)
		return c, err
	})
	if err != nil {
		return clientdetail.Detail{}, fmt.Errorf("get client %q: consultations: %w", clientID, err)
	}

	const casesQ = `SELECT id, description, category, status, fee, paid_amount, created_at FROM cases
		WHERE client_id = @id ORDER BY created_at DESC`
	caseRows, err := r.db.Query(ctx, casesQ, pgx.NamedArgs{"id": clientID})
	if err != nil {
		return clientdetail.Detail{}, fmt.Errorf("get client %q: cases: %w", clientID, err)
	}
	d.Cases, err = pgx.CollectRows(caseRows, func(row pgx.CollectableRow) (clientdetail.Case, error) {
		var c clientdetail.Case
		err := row.Scan(&c.ID, &c.Description, &c.Category, &c.Status, &c.Fee, &c.Paid, &c.CreatedAt)
		return c, err
	})
	if err != nil {
		return clientdetail.Detail{}, fmt.Errorf("get client %q: cases: %w", clientID, err)
	}
	for _, c := range d.Cases {
		d.RevenueTotal += c.Paid
	}

	const notesQ = `SELECT id, text, created_by, created_at FROM client_notes
		WHERE client_id = @id ORDER BY created_at DESC`
	noteRows, err := r.db.Query(ctx, notesQ, pgx.NamedArgs{"id": clientID})
	if err != nil {
		return clientdetail.Detail{}, fmt.Errorf("get client %q: notes: %w", clientID, err)
	}
	d.Notes, err = pgx.CollectRows(noteRows, func(row pgx.CollectableRow) (clientdetail.Note, error) {
		var n clientdetail.Note
		err := row.Scan(&n.ID, &n.Text, &n.CreatedBy, &n.CreatedAt)
		return n, err
	})
	if err != nil {
		return clientdetail.Detail{}, fmt.Errorf("get client %q: notes: %w", clientID, err)
	}

	return d, nil
}

func (r *ClientDetailRepository) AddNote(ctx context.Context, clientID, text, createdBy string) (clientdetail.Note, error) {
	const q = `INSERT INTO client_notes (client_id, text, created_by)
		VALUES (@client_id, @text, @created_by)
		RETURNING id, text, created_by, created_at`

	var n clientdetail.Note
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"client_id":  clientID,
		"text":       text,
		"created_by": createdBy,
	}).Scan(&n.ID, &n.Text, &n.CreatedBy, &n.CreatedAt)
	if err != nil {
		return clientdetail.Note{}, fmt.Errorf("add note for client %q: %w", clientID, err)
	}
	return n, nil
}
