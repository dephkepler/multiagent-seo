package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/clientdetail"
	"multiagent-seo/internal/domain/consultations"
)

type ClientDetailRepository struct {
	db *pgxpool.Pool
	// encryptionKey decrypts address/birthdate/tax id via pgcrypto — see
	// Get. Must match ConsultationRepository.UpdateClient's key, since
	// that's what encrypted them.
	encryptionKey string
}

func NewClientDetailRepository(db *pgxpool.Pool, encryptionKey string) *ClientDetailRepository {
	return &ClientDetailRepository{db: db, encryptionKey: encryptionKey}
}

// Get reads the client row plus its full history in four separate
// queries — one client card is looked up rarely enough (a staff member
// opening one at a time) that four simple queries beat one join sprawling
// across leads/consultations/cases/client_notes with mismatched row counts.
func (r *ClientDetailRepository) Get(ctx context.Context, clientID string) (clientdetail.Detail, error) {
	var d clientdetail.Detail

	const clientQ = `SELECT id, name, last_name, first_name, patronymic, gender, coalesce(phone, ''), email,
			client_type, company_name, company_code,
			coalesce(pgp_sym_decrypt(address_enc, @enc_key), ''),
			coalesce(pgp_sym_decrypt(birthdate_enc, @enc_key), ''),
			coalesce(pgp_sym_decrypt(tax_id_enc, @enc_key), ''),
			first_seen_at, last_seen_at
		FROM clients WHERE id = @id`
	err := r.db.QueryRow(ctx, clientQ, pgx.NamedArgs{"id": clientID, "enc_key": r.encryptionKey}).Scan(
		&d.Client.ID, &d.Client.Name, &d.Client.LastName, &d.Client.FirstName, &d.Client.Patronymic,
		&d.Client.Gender, &d.Client.Phone, &d.Client.Email,
		&d.Client.ClientType, &d.Client.CompanyName, &d.Client.CompanyCode,
		&d.Client.Address, &d.Client.Birthdate, &d.Client.TaxID,
		&d.Client.FirstSeenAt, &d.Client.LastSeenAt,
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
	// Consultation revenue is the other half of a client's lifetime value —
	// a client who never signed a case but paid for consultations still
	// brought in real money. Same price>0/completed filter as leadstats, so
	// this figure and the admin dashboard's "Заработано" agree on what
	// counts.
	for _, c := range d.Consultations {
		if c.Price > 0 && c.Status == consultations.StatusCompleted {
			d.RevenueTotal += c.Price
		}
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

// Delete refuses (ErrHasHistory) if the client has any lead, consultation,
// or case — those are real business records (money, audit trail), not
// something a stray delete click should be able to destroy. client_notes
// don't block deletion — they're staff's own scratch log about this client,
// worthless once the client itself is gone, so they're cleaned up here
// rather than treated as "history".
func (r *ClientDetailRepository) Delete(ctx context.Context, clientID string) error {
	var leads, consults, cases int
	const countQ = `SELECT
		(SELECT count(*) FROM leads WHERE client_id = @id),
		(SELECT count(*) FROM consultations WHERE client_id = @id),
		(SELECT count(*) FROM cases WHERE client_id = @id)`
	if err := r.db.QueryRow(ctx, countQ, pgx.NamedArgs{"id": clientID}).Scan(&leads, &consults, &cases); err != nil {
		return fmt.Errorf("delete client %q: count history: %w", clientID, err)
	}
	if leads > 0 || consults > 0 || cases > 0 {
		return clientdetail.ErrHasHistory
	}

	const notesQ = `DELETE FROM client_notes WHERE client_id = @id`
	if _, err := r.db.Exec(ctx, notesQ, pgx.NamedArgs{"id": clientID}); err != nil {
		return fmt.Errorf("delete client %q: notes: %w", clientID, err)
	}

	const clientQ = `DELETE FROM clients WHERE id = @id`
	tag, err := r.db.Exec(ctx, clientQ, pgx.NamedArgs{"id": clientID})
	if err != nil {
		return fmt.Errorf("delete client %q: %w", clientID, err)
	}
	if tag.RowsAffected() == 0 {
		return clientdetail.ErrNotFound
	}
	return nil
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
