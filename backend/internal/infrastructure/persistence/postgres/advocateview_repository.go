package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/advocateview"
)

// AdvocateViewRepository is the only adapter an advocate's requests reach. It
// deliberately repeats a little SQL that ClientDetailRepository also has
// (notes, consultations) instead of reusing it: those queries decrypt address,
// birthdate and РНОКПП and count firm-wide revenue, and the guarantee worth
// having here is textual — no query in this file names an encrypted column or
// a row the owner predicate does not cover.
type AdvocateViewRepository struct {
	db *pgxpool.Pool
}

func NewAdvocateViewRepository(db *pgxpool.Pool) *AdvocateViewRepository {
	return &AdvocateViewRepository{db: db}
}

// ownedCase is the one definition of "mine" — the roster link, or, for cases
// opened before the roster existed, an exact surname match on a case that has
// no link at all. It mirrors FinanceRepository.AdvocateCollections; if the two
// drifted apart, an advocate's settlement would count money from a case their
// own case list refuses to show. The empty-name guard matters: plenty of rows
// have advocate_name = ”, and without it an advocate with a blank roster name
// would own all of them.
const ownedCase = `(
	c.advocate_id = @advocate_id::uuid
	OR (c.advocate_id IS NULL AND @advocate_name <> '' AND c.advocate_name = @advocate_name)
)`

func ownerArgs(owner advocateview.Owner) pgx.NamedArgs {
	return pgx.NamedArgs{"advocate_id": owner.ID, "advocate_name": owner.FullName}
}

func (r *AdvocateViewRepository) Advocate(ctx context.Context, advocateID string) (advocateview.Advocate, error) {
	const q = `SELECT id::text, full_name, is_active, commission_percent FROM advocates WHERE id = @id::uuid`

	var a advocateview.Advocate
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": advocateID}).
		Scan(&a.ID, &a.FullName, &a.IsActive, &a.CommissionPercent)
	if errors.Is(err, pgx.ErrNoRows) {
		return advocateview.Advocate{}, advocateview.ErrNotFound
	}
	if err != nil {
		return advocateview.Advocate{}, fmt.Errorf("advocate %q: %w", advocateID, err)
	}
	return a, nil
}

func (r *AdvocateViewRepository) ListCases(ctx context.Context, owner advocateview.Owner) ([]advocateview.Case, error) {
	const q = `
		SELECT c.id::text, c.client_id::text, cl.name, coalesce(cl.phone, ''),
			c.category, c.status, c.description, c.fee, c.paid_amount, c.created_at
		FROM cases c
		JOIN clients cl ON cl.id = c.client_id
		WHERE ` + ownedCase + `
		ORDER BY c.created_at DESC`

	rows, err := r.db.Query(ctx, q, ownerArgs(owner))
	if err != nil {
		return nil, fmt.Errorf("advocate cases: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanAdvocateCase)
	if err != nil {
		return nil, fmt.Errorf("advocate cases: %w", err)
	}
	if err := r.attachPayments(ctx, list); err != nil {
		return nil, err
	}
	return list, nil
}

// attachPayments fills the installment ledger for a set of cases in one query —
// the advocate's list is what they chase clients with, and "3 000 of 12 000
// paid" is not enough to make that call without the dates.
func (r *AdvocateViewRepository) attachPayments(ctx context.Context, list []advocateview.Case) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]string, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.ID)
	}

	const q = `SELECT case_id::text, id::text, amount, paid_at FROM case_payments
		WHERE case_id = ANY(@ids::uuid[]) ORDER BY paid_at, id`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"ids": ids})
	if err != nil {
		return fmt.Errorf("advocate case payments: %w", err)
	}
	defer rows.Close()

	perCase := map[string][]advocateview.Payment{}
	for rows.Next() {
		var caseID string
		var p advocateview.Payment
		if err := rows.Scan(&caseID, &p.ID, &p.Amount, &p.PaidAt); err != nil {
			return fmt.Errorf("advocate case payments: scan: %w", err)
		}
		perCase[caseID] = append(perCase[caseID], p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("advocate case payments: %w", err)
	}
	for i := range list {
		list[i].Payments = perCase[list[i].ID]
	}
	return nil
}

func (r *AdvocateViewRepository) ListClients(ctx context.Context, owner advocateview.Owner) ([]advocateview.Client, error) {
	const q = `
		SELECT cl.id::text, cl.name, coalesce(cl.phone, ''),
			count(*), sum(c.fee), sum(c.paid_amount), max(c.created_at)
		FROM cases c
		JOIN clients cl ON cl.id = c.client_id
		WHERE ` + ownedCase + `
		GROUP BY cl.id, cl.name, cl.phone
		ORDER BY max(c.created_at) DESC`

	rows, err := r.db.Query(ctx, q, ownerArgs(owner))
	if err != nil {
		return nil, fmt.Errorf("advocate clients: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (advocateview.Client, error) {
		var c advocateview.Client
		err := row.Scan(&c.ID, &c.Name, &c.Phone, &c.Cases, &c.Fee, &c.Paid, &c.LastCaseAt)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("advocate clients: %w", err)
	}
	return list, nil
}

func (r *AdvocateViewRepository) GetClient(ctx context.Context, owner advocateview.Owner, clientID string) (advocateview.Card, error) {
	args := ownerArgs(owner)
	args["client_id"] = clientID

	// The client row is fetched through the same owner predicate rather than
	// by id, so a client the advocate has no case with is not merely filtered
	// out downstream — it never loads. Note the columns: name and phone only.
	const clientQ = `
		SELECT cl.id::text, cl.name, coalesce(cl.phone, ''),
			count(*), sum(c.fee), sum(c.paid_amount), max(c.created_at)
		FROM cases c
		JOIN clients cl ON cl.id = c.client_id
		WHERE cl.id = @client_id::uuid AND ` + ownedCase + `
		GROUP BY cl.id, cl.name, cl.phone`

	var card advocateview.Card
	err := r.db.QueryRow(ctx, clientQ, args).Scan(
		&card.Client.ID, &card.Client.Name, &card.Client.Phone,
		&card.Client.Cases, &card.Client.Fee, &card.Client.Paid, &card.Client.LastCaseAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return advocateview.Card{}, advocateview.ErrNotFound
	}
	if err != nil {
		return advocateview.Card{}, fmt.Errorf("advocate client %q: %w", clientID, err)
	}

	const casesQ = `
		SELECT c.id::text, c.client_id::text, cl.name, coalesce(cl.phone, ''),
			c.category, c.status, c.description, c.fee, c.paid_amount, c.created_at
		FROM cases c
		JOIN clients cl ON cl.id = c.client_id
		WHERE cl.id = @client_id::uuid AND ` + ownedCase + `
		ORDER BY c.created_at DESC`

	caseRows, err := r.db.Query(ctx, casesQ, args)
	if err != nil {
		return advocateview.Card{}, fmt.Errorf("advocate client %q: cases: %w", clientID, err)
	}
	card.Cases, err = pgx.CollectRows(caseRows, scanAdvocateCase)
	if err != nil {
		return advocateview.Card{}, fmt.Errorf("advocate client %q: cases: %w", clientID, err)
	}
	if err := r.attachPayments(ctx, card.Cases); err != nil {
		return advocateview.Card{}, err
	}

	const consultQ = `SELECT id::text, scheduled_at, price, status, case_note FROM consultations
		WHERE client_id = @client_id::uuid ORDER BY scheduled_at DESC`

	consultRows, err := r.db.Query(ctx, consultQ, pgx.NamedArgs{"client_id": clientID})
	if err != nil {
		return advocateview.Card{}, fmt.Errorf("advocate client %q: consultations: %w", clientID, err)
	}
	card.Consultations, err = pgx.CollectRows(consultRows, func(row pgx.CollectableRow) (advocateview.Consultation, error) {
		var c advocateview.Consultation
		err := row.Scan(&c.ID, &c.ScheduledAt, &c.Price, &c.Status, &c.CaseNote)
		return c, err
	})
	if err != nil {
		return advocateview.Card{}, fmt.Errorf("advocate client %q: consultations: %w", clientID, err)
	}

	const notesQ = `SELECT id::text, text, created_by, created_at FROM client_notes
		WHERE client_id = @client_id::uuid ORDER BY created_at DESC`

	noteRows, err := r.db.Query(ctx, notesQ, pgx.NamedArgs{"client_id": clientID})
	if err != nil {
		return advocateview.Card{}, fmt.Errorf("advocate client %q: notes: %w", clientID, err)
	}
	card.Notes, err = pgx.CollectRows(noteRows, func(row pgx.CollectableRow) (advocateview.Note, error) {
		var n advocateview.Note
		err := row.Scan(&n.ID, &n.Text, &n.CreatedBy, &n.CreatedAt)
		return n, err
	})
	if err != nil {
		return advocateview.Card{}, fmt.Errorf("advocate client %q: notes: %w", clientID, err)
	}
	return card, nil
}

// AddNote checks ownership inside the INSERT: the SELECT that produces the row
// is the owner predicate, so a client the advocate has no case with inserts
// nothing at all rather than being rejected by a prior lookup that could go
// stale between the two statements.
func (r *AdvocateViewRepository) AddNote(
	ctx context.Context,
	owner advocateview.Owner,
	clientID, text, createdBy string,
) (advocateview.Note, error) {
	const q = `
		INSERT INTO client_notes (client_id, text, created_by)
		SELECT @client_id::uuid, @text, @created_by
		WHERE EXISTS (
			SELECT 1 FROM cases c
			WHERE c.client_id = @client_id::uuid AND ` + ownedCase + `
		)
		RETURNING id::text, text, created_by, created_at`

	args := ownerArgs(owner)
	args["client_id"] = clientID
	args["text"] = text
	args["created_by"] = createdBy

	var n advocateview.Note
	err := r.db.QueryRow(ctx, q, args).Scan(&n.ID, &n.Text, &n.CreatedBy, &n.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return advocateview.Note{}, advocateview.ErrNotFound
	}
	if err != nil {
		return advocateview.Note{}, fmt.Errorf("advocate note for client %q: %w", clientID, err)
	}
	return n, nil
}

func (r *AdvocateViewRepository) UpdateCaseStatus(
	ctx context.Context,
	owner advocateview.Owner,
	caseID, status string,
) error {
	const q = `UPDATE cases c SET status = @status WHERE c.id = @case_id::uuid AND ` + ownedCase

	args := ownerArgs(owner)
	args["case_id"] = caseID
	args["status"] = status

	tag, err := r.db.Exec(ctx, q, args)
	if err != nil {
		return fmt.Errorf("advocate case %q status: %w", caseID, err)
	}
	if tag.RowsAffected() == 0 {
		return advocateview.ErrNotFound
	}
	return nil
}

func (r *AdvocateViewRepository) CollectionsByMonth(ctx context.Context, owner advocateview.Owner) ([]advocateview.MonthMoney, error) {
	// paid_at is a date, not a timestamp, so the month is whatever day staff
	// recorded — no timezone conversion to get wrong here.
	const q = `
		SELECT to_char(date_trunc('month', p.paid_at), 'YYYY-MM'), sum(p.amount)
		FROM cases c
		JOIN case_payments p ON p.case_id = c.id
		WHERE ` + ownedCase + `
		GROUP BY 1
		ORDER BY 1`

	rows, err := r.db.Query(ctx, q, ownerArgs(owner))
	if err != nil {
		return nil, fmt.Errorf("advocate collections by month: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (advocateview.MonthMoney, error) {
		var m advocateview.MonthMoney
		err := row.Scan(&m.Month, &m.Collected)
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("advocate collections by month: %w", err)
	}
	return list, nil
}

// PaidOut counts only payouts whose external_ref names this advocate — the same
// rule as FinanceRepository.AdvocatePayouts. Lump sums entered by hand name
// nobody and are not silently credited here, which is why the settlement
// carries PaidIsPartial.
func (r *AdvocateViewRepository) PaidOut(ctx context.Context, advocateID string) (float64, error) {
	const q = `
		SELECT coalesce(sum(amount), 0) FROM expenses
		WHERE status = 'posted' AND category_code = 'advocates'
			AND external_ref LIKE 'advocate:' || @advocate_id || ':%'`

	var paid float64
	if err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"advocate_id": advocateID}).Scan(&paid); err != nil {
		return 0, fmt.Errorf("advocate payouts %q: %w", advocateID, err)
	}
	return paid, nil
}

func scanAdvocateCase(row pgx.CollectableRow) (advocateview.Case, error) {
	var c advocateview.Case
	err := row.Scan(
		&c.ID, &c.ClientID, &c.ClientName, &c.ClientPhone,
		&c.Category, &c.Status, &c.Description, &c.Fee, &c.Paid, &c.CreatedAt,
	)
	return c, err
}
