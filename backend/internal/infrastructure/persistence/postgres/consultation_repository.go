package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/consultations"
)

type ConsultationRepository struct {
	db *pgxpool.Pool
	// encryptionKey encrypts sensitive ClientEdit fields (address/
	// birthdate/tax id) at rest via pgcrypto — see UpdateClient.
	encryptionKey string
}

func NewConsultationRepository(db *pgxpool.Pool, encryptionKey string) *ConsultationRepository {
	return &ConsultationRepository{db: db, encryptionKey: encryptionKey}
}

func (r *ConsultationRepository) FindClient(ctx context.Context, clientID string) (consultations.Client, error) {
	const q = `SELECT id, name, coalesce(phone, ''), coalesce(telegram_name, ''), coalesce(telegram_chat_id, 0)
		FROM clients WHERE id = @id`
	var c consultations.Client
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": clientID}).
		Scan(&c.ID, &c.Name, &c.Phone, &c.TelegramName, &c.TelegramChatID)
	if err != nil {
		return consultations.Client{}, fmt.Errorf("find client %q: %w", clientID, err)
	}
	return c, nil
}

func (r *ConsultationRepository) SearchClients(ctx context.Context, query string) ([]consultations.Client, error) {
	const q = `SELECT id, name, coalesce(phone, ''), coalesce(telegram_name, '')
		FROM clients
		WHERE name ILIKE '%' || @query || '%'
			OR telegram_name ILIKE '%' || @query || '%'
			OR phone ILIKE '%' || @query || '%'
		ORDER BY last_seen_at DESC
		LIMIT 8`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"query": query})
	if err != nil {
		return nil, fmt.Errorf("search clients %q: %w", query, err)
	}
	defer rows.Close()

	var out []consultations.Client
	for rows.Next() {
		var c consultations.Client
		if err := rows.Scan(&c.ID, &c.Name, &c.Phone, &c.TelegramName); err != nil {
			return nil, fmt.Errorf("search clients %q: scan: %w", query, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search clients %q: %w", query, err)
	}
	return out, nil
}

// CreateClient finds-or-creates a client. With a phone, it's the natural
// identity key — same idempotent upsert-by-phone as webleads.ResolveClient,
// so re-entering the same phone twice reuses one row. Without one (an old
// client whose only contact on file is an email/Telegram handle, or
// nothing at all yet), there's no reliable key to match an existing row
// against, so this always inserts a fresh client — the partial unique
// index on phone (see migration 000040) only applies to non-empty phones,
// so any number of "no phone" clients can coexist without colliding.
func (r *ConsultationRepository) CreateClient(ctx context.Context, name, phone, email, telegramName string) (consultations.Client, error) {
	// last_name/first_name/patronymic are the client card's structured
	// fields — name arrives as one free-text string here, so it's split
	// once at creation instead of leaving those columns blank forever (see
	// SplitName). Only set on a fresh INSERT: the ON CONFLICT branch below
	// never touches them, so a manual correction made later on the client
	// card is never overwritten by a repeat /book with the same phone.
	lastName, firstName, patronymic := consultations.SplitName(name)

	if phone != "" {
		// The ON CONFLICT target's WHERE clause has to repeat the partial
		// unique index's predicate verbatim (see migration 000040) —
		// Postgres only matches ON CONFLICT against a partial index when
		// the inference clause matches its WHERE exactly; without this,
		// every insert here fails with "no unique or exclusion constraint
		// matching the ON CONFLICT specification", which is exactly what
		// happened between that migration landing and this fix.
		const q = `INSERT INTO clients (phone, name, last_name, first_name, patronymic)
			VALUES (@phone, @name, @last_name, @first_name, @patronymic)
			ON CONFLICT (phone) WHERE phone IS NOT NULL AND phone <> '' DO UPDATE SET
				last_seen_at = now(),
				name = CASE WHEN @name = '' THEN clients.name ELSE @name END
			RETURNING id, name, coalesce(phone, ''), coalesce(telegram_name, '')`
		var c consultations.Client
		err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
			"phone": phone, "name": name,
			"last_name": lastName, "first_name": firstName, "patronymic": patronymic,
		}).Scan(&c.ID, &c.Name, &c.Phone, &c.TelegramName)
		if err != nil {
			return consultations.Client{}, fmt.Errorf("create client %q: %w", name, err)
		}
		return c, nil
	}

	const q = `INSERT INTO clients (name, email, telegram_name, last_name, first_name, patronymic)
		VALUES (@name, @email, @telegram_name, @last_name, @first_name, @patronymic)
		RETURNING id, name, coalesce(phone, ''), coalesce(telegram_name, '')`
	var c consultations.Client
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"name": name, "email": email, "telegram_name": telegramName,
		"last_name": lastName, "first_name": firstName, "patronymic": patronymic,
	}).Scan(&c.ID, &c.Name, &c.Phone, &c.TelegramName)
	if err != nil {
		return consultations.Client{}, fmt.Errorf("create client %q: %w", name, err)
	}
	return c, nil
}

// SetClientTelegram moves the binding rather than adding one: a chat id is now
// what a Mini App request is authenticated against, and 000057 made it unique,
// so it can only ever point at one client.
//
// The same human ends up with two client rows whenever they file a second
// intake under a different phone — clients are keyed by phone — and the chat
// follows the row they just used, which is the case actually being opened. Two
// statements in a transaction, not a data-modifying CTE: Postgres does not
// order the sub-statements of a CTE against the main query, so releasing the
// old row and taking the id could be attempted in either order and trip the
// unique index.
func (r *ConsultationRepository) SetClientTelegram(ctx context.Context, clientID string, chatID int64, telegramName string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("set telegram info for client %q: %w", clientID, err)
	}
	defer tx.Rollback(ctx)

	const release = `UPDATE clients SET telegram_chat_id = NULL WHERE telegram_chat_id = @chat_id AND id <> @id`
	if _, err := tx.Exec(ctx, release, pgx.NamedArgs{"chat_id": chatID, "id": clientID}); err != nil {
		return fmt.Errorf("release telegram chat %d: %w", chatID, err)
	}

	const bind = `UPDATE clients SET telegram_chat_id = @chat_id, telegram_name = @telegram_name WHERE id = @id`
	tag, err := tx.Exec(ctx, bind, pgx.NamedArgs{"chat_id": chatID, "telegram_name": telegramName, "id": clientID})
	if err != nil {
		return fmt.Errorf("set telegram info for client %q: %w", clientID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set telegram info: no client with id %q", clientID)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("set telegram info for client %q: commit: %w", clientID, err)
	}
	return nil
}

func (r *ConsultationRepository) SetClientEmail(ctx context.Context, clientID, email string) error {
	const q = `UPDATE clients SET email = @email WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"email": email, "id": clientID})
	if err != nil {
		return fmt.Errorf("set client email %q: %w", clientID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set client email: no client with id %q", clientID)
	}
	return nil
}

// SetClientNamePart writes one column and recomputes name from it plus
// whatever is already in the other two -- a literal SQL statement per part
// (not a dynamic column name built from user input) so the query stays a
// fixed, reviewable string. Each piece goes through nullif first, turning
// a blank value into SQL NULL, before concat_ws: unlike ComposeName in Go
// (which only joins non-blank parts), plain concat_ws does not skip blank
// arguments, only NULLs -- a blank part would otherwise leave a double
// space in name (concat_ws still places a separator around a blank piece,
// just not around a NULL one).
func (r *ConsultationRepository) SetClientNamePart(ctx context.Context, clientID string, part consultations.ClientNamePart, value string) error {
	var q string
	switch part {
	case consultations.NamePartLast:
		q = `UPDATE clients SET last_name = @value,
			name = concat_ws(' ', nullif(@value, ''), nullif(first_name, ''), nullif(patronymic, ''))
			WHERE id = @id`
	case consultations.NamePartFirst:
		q = `UPDATE clients SET first_name = @value,
			name = concat_ws(' ', nullif(last_name, ''), nullif(@value, ''), nullif(patronymic, ''))
			WHERE id = @id`
	case consultations.NamePartPatronymic:
		q = `UPDATE clients SET patronymic = @value,
			name = concat_ws(' ', nullif(last_name, ''), nullif(first_name, ''), nullif(@value, ''))
			WHERE id = @id`
	default:
		return fmt.Errorf("set client name part: unknown part %q", part)
	}
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"value": value, "id": clientID})
	if err != nil {
		return fmt.Errorf("set client name part %q for %q: %w", part, clientID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set client name part: no client with id %q", clientID)
	}
	return nil
}

func (r *ConsultationRepository) SetClientPhone(ctx context.Context, clientID, phone string) error {
	const q = `UPDATE clients SET phone = @phone WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"phone": phone, "id": clientID})
	if err != nil {
		return fmt.Errorf("set client phone %q: %w", clientID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set client phone: no client with id %q", clientID)
	}
	return nil
}

// UpdateClient writes every field the client card can edit. Address/
// Birthdate/TaxID go in encrypted (pgp_sym_encrypt, keyed by
// r.encryptionKey) — an empty value stores NULL rather than encrypting an
// empty string, so "not recorded" stays distinguishable from "recorded as
// blank".
func (r *ConsultationRepository) UpdateClient(ctx context.Context, clientID string, edit consultations.ClientEdit) error {
	const q = `UPDATE clients SET
			name = @name, last_name = @last_name, first_name = @first_name, patronymic = @patronymic,
			phone = @phone, gender = @gender, email = @email,
			client_type = @client_type, company_name = @company_name, company_code = @company_code,
			address_enc = CASE WHEN @address = '' THEN NULL ELSE pgp_sym_encrypt(@address, @enc_key) END,
			birthdate_enc = CASE WHEN @birthdate = '' THEN NULL ELSE pgp_sym_encrypt(@birthdate, @enc_key) END,
			tax_id_enc = CASE WHEN @tax_id = '' THEN NULL ELSE pgp_sym_encrypt(@tax_id, @enc_key) END
		WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"name":         consultations.ComposeName(edit.LastName, edit.FirstName, edit.Patronymic),
		"last_name":    edit.LastName,
		"first_name":   edit.FirstName,
		"patronymic":   edit.Patronymic,
		"phone":        edit.Phone,
		"gender":       edit.Gender,
		"email":        edit.Email,
		"client_type":  edit.ClientType,
		"company_name": edit.CompanyName,
		"company_code": edit.CompanyCode,
		"address":      edit.Address,
		"birthdate":    edit.Birthdate,
		"tax_id":       edit.TaxID,
		"enc_key":      r.encryptionKey,
		"id":           clientID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_clients_phone" {
			return fmt.Errorf("update client %q: %w", clientID, consultations.ErrPhoneInUse)
		}
		return fmt.Errorf("update client %q: %w", clientID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update client: no client with id %q", clientID)
	}
	return nil
}

func (r *ConsultationRepository) Save(ctx context.Context, c consultations.Consultation) (consultations.Consultation, error) {
	if c.Status == "" {
		c.Status = consultations.StatusScheduled
	}
	const q = `INSERT INTO consultations (client_id, scheduled_at, price, case_note, created_by, status)
		VALUES (@client_id, @scheduled_at, @price, @case_note, @created_by, @status)
		RETURNING id`

	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"client_id":    c.ClientID,
		"scheduled_at": c.ScheduledAt,
		"price":        c.Price,
		"case_note":    c.CaseNote,
		"created_by":   c.CreatedBy,
		"status":       c.Status,
	}).Scan(&c.ID)
	if err != nil {
		return consultations.Consultation{}, fmt.Errorf("save consultation for client %q: %w", c.ClientID, err)
	}
	return c, nil
}

func (r *ConsultationRepository) LatestConsultation(ctx context.Context, clientID string) (consultations.Consultation, error) {
	const q = `SELECT id, client_id, scheduled_at, price, case_note, created_by, status
		FROM consultations WHERE client_id = @client_id
		ORDER BY created_at DESC LIMIT 1`
	var c consultations.Consultation
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"client_id": clientID}).Scan(
		&c.ID, &c.ClientID, &c.ScheduledAt, &c.Price, &c.CaseNote, &c.CreatedBy, &c.Status,
	)
	if err != nil {
		return consultations.Consultation{}, fmt.Errorf("latest consultation for client %q: %w", clientID, err)
	}
	return c, nil
}

// UpdateStatus is how staff mark a consultation's outcome — driven by the
// inline buttons on the booking confirmation the bot sends (see
// AdminBot.handleCallback), not by anything in the admin web UI.
func (r *ConsultationRepository) UpdateStatus(ctx context.Context, consultationID, status string) error {
	const q = `UPDATE consultations SET status = @status WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"status": status, "id": consultationID})
	if err != nil {
		return fmt.Errorf("update consultation status %q: %w", consultationID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update consultation status: no consultation with id %q", consultationID)
	}
	return nil
}

// MarkCompleted writes the status and the payment answer together: they come
// from one tap in the bot, and a half-applied pair would leave revenue that the
// P&L counts with nobody knowing whether it arrived.
func (r *ConsultationRepository) MarkCompleted(ctx context.Context, consultationID string, paid bool, paidAt time.Time) error {
	const q = `UPDATE consultations
		SET status = 'completed', paid = @paid, paid_at = CASE WHEN @paid::boolean THEN @paid_at::date ELSE NULL END
		WHERE id = @id`

	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"paid": paid, "paid_at": paidAt, "id": consultationID})
	if err != nil {
		return fmt.Errorf("mark consultation completed %q: %w", consultationID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mark consultation completed: no consultation with id %q", consultationID)
	}
	return nil
}

// CreateAdvocate always inserts a new row — advocates are a roster (see
// ports.go), not a single slot.
func (r *ConsultationRepository) CreateAdvocate(ctx context.Context, fullName string) (consultations.Advocate, error) {
	const q = `INSERT INTO advocates (full_name) VALUES (@full_name)
		RETURNING id, full_name, coalesce(telegram_username, ''), coalesce(telegram_chat_id, 0), is_active`
	a, err := scanAdvocate(r.db.QueryRow(ctx, q, pgx.NamedArgs{"full_name": fullName}))
	if err != nil {
		return consultations.Advocate{}, fmt.Errorf("create advocate: %w", err)
	}
	return a, nil
}

func (r *ConsultationRepository) ListAdvocates(ctx context.Context, activeOnly bool) ([]consultations.Advocate, error) {
	q := `SELECT id, full_name, coalesce(telegram_username, ''), coalesce(telegram_chat_id, 0), is_active
		FROM advocates`
	if activeOnly {
		q += ` WHERE is_active`
	}
	q += ` ORDER BY full_name`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list advocates: %w", err)
	}
	defer rows.Close()

	var advocates []consultations.Advocate
	for rows.Next() {
		a, err := scanAdvocate(rows)
		if err != nil {
			return nil, fmt.Errorf("list advocates: scan: %w", err)
		}
		advocates = append(advocates, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list advocates: %w", err)
	}
	return advocates, nil
}

func (r *ConsultationRepository) DeactivateAdvocate(ctx context.Context, advocateID string) error {
	const q = `UPDATE advocates SET is_active = false WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": advocateID})
	if err != nil {
		return fmt.Errorf("deactivate advocate %q: %w", advocateID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deactivate advocate: no advocate with id %q", advocateID)
	}
	return nil
}

func (r *ConsultationRepository) SetAdvocateTelegram(ctx context.Context, advocateID string, chatID int64, telegramName string) error {
	const q = `UPDATE advocates SET telegram_chat_id = @chat_id, telegram_username = @telegram_name WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"chat_id": chatID, "telegram_name": telegramName, "id": advocateID})
	if err != nil {
		return fmt.Errorf("set advocate telegram %q: %w", advocateID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set advocate telegram: no advocate with id %q", advocateID)
	}
	return nil
}

// GetAdvocate is a stopgap for the reminder loop (see ports.go) — first
// active advocate, arbitrarily, since reminders aren't per-advocate yet.
func (r *ConsultationRepository) GetAdvocate(ctx context.Context) (consultations.Advocate, error) {
	const q = `SELECT id, full_name, coalesce(telegram_username, ''), coalesce(telegram_chat_id, 0), is_active
		FROM advocates WHERE is_active ORDER BY created_at LIMIT 1`
	a, err := scanAdvocate(r.db.QueryRow(ctx, q))
	if err != nil {
		return consultations.Advocate{}, fmt.Errorf("get advocate: %w", err)
	}
	return a, nil
}

func scanAdvocate(row pgx.Row) (consultations.Advocate, error) {
	var a consultations.Advocate
	err := row.Scan(&a.ID, &a.FullName, &a.TelegramUsername, &a.TelegramChatID, &a.IsActive)
	return a, err
}

func (r *ConsultationRepository) DueClientReminders(ctx context.Context, before time.Duration) ([]consultations.ReminderTarget, error) {
	const q = `SELECT c.id, c.client_id, c.scheduled_at, c.price, c.case_note, c.created_by,
			cl.id, cl.name, coalesce(cl.phone, ''), coalesce(cl.telegram_name, ''), coalesce(cl.telegram_chat_id, 0)
		FROM consultations c
		JOIN clients cl ON cl.id = c.client_id
		WHERE c.client_reminder_sent_at IS NULL
			AND c.scheduled_at > now()
			AND c.scheduled_at <= @deadline
			AND cl.telegram_chat_id IS NOT NULL`

	return queryReminderTargets(ctx, r.db, q, time.Now().Add(before))
}

func (r *ConsultationRepository) DueAdvocateReminders(ctx context.Context, before time.Duration) ([]consultations.ReminderTarget, error) {
	const q = `SELECT c.id, c.client_id, c.scheduled_at, c.price, c.case_note, c.created_by,
			cl.id, cl.name, coalesce(cl.phone, ''), coalesce(cl.telegram_name, ''), coalesce(cl.telegram_chat_id, 0)
		FROM consultations c
		JOIN clients cl ON cl.id = c.client_id
		WHERE c.reminder_sent_at IS NULL
			AND c.scheduled_at > now()
			AND c.scheduled_at <= @deadline`

	return queryReminderTargets(ctx, r.db, q, time.Now().Add(before))
}

func queryReminderTargets(ctx context.Context, db *pgxpool.Pool, q string, deadline time.Time) ([]consultations.ReminderTarget, error) {
	rows, err := db.Query(ctx, q, pgx.NamedArgs{"deadline": deadline})
	if err != nil {
		return nil, fmt.Errorf("due reminders: %w", err)
	}
	defer rows.Close()

	var targets []consultations.ReminderTarget
	for rows.Next() {
		var t consultations.ReminderTarget
		if err := rows.Scan(
			&t.Consultation.ID, &t.Consultation.ClientID, &t.Consultation.ScheduledAt, &t.Consultation.Price, &t.Consultation.CaseNote, &t.Consultation.CreatedBy,
			&t.Client.ID, &t.Client.Name, &t.Client.Phone, &t.Client.TelegramName, &t.Client.TelegramChatID,
		); err != nil {
			return nil, fmt.Errorf("due reminders: scan: %w", err)
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("due reminders: %w", err)
	}
	return targets, nil
}

func (r *ConsultationRepository) MarkClientReminderSent(ctx context.Context, consultationID string) error {
	const q = `UPDATE consultations SET client_reminder_sent_at = now() WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": consultationID})
	if err != nil {
		return fmt.Errorf("mark client reminder sent for consultation %q: %w", consultationID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mark client reminder sent: no consultation with id %q", consultationID)
	}
	return nil
}

func (r *ConsultationRepository) MarkReminderSent(ctx context.Context, consultationID string) error {
	const q = `UPDATE consultations SET reminder_sent_at = now() WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": consultationID})
	if err != nil {
		return fmt.Errorf("mark advocate reminder sent for consultation %q: %w", consultationID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("mark advocate reminder sent: no consultation with id %q", consultationID)
	}
	return nil
}

func (r *ConsultationRepository) GetStaffLanguage(ctx context.Context, telegramUserID int64) (consultations.StaffLang, error) {
	const q = `SELECT language FROM staff_prefs WHERE telegram_user_id = @id`
	var lang string
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": telegramUserID}).Scan(&lang)
	if errors.Is(err, pgx.ErrNoRows) {
		return consultations.StaffLangUK, nil
	}
	if err != nil {
		return "", fmt.Errorf("get staff language for %d: %w", telegramUserID, err)
	}
	return consultations.StaffLang(lang), nil
}

func (r *ConsultationRepository) SetStaffLanguage(ctx context.Context, telegramUserID int64, lang consultations.StaffLang) error {
	const q = `INSERT INTO staff_prefs (telegram_user_id, language) VALUES (@id, @lang)
		ON CONFLICT (telegram_user_id) DO UPDATE SET language = @lang, updated_at = now()`
	_, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": telegramUserID, "lang": string(lang)})
	if err != nil {
		return fmt.Errorf("set staff language for %d: %w", telegramUserID, err)
	}
	return nil
}

// HeldSlots reads the starts a client may not pick. Bounded by the picker's own
// window rather than "everything upcoming": the query runs on every launch of
// the Mini App, and the index from 000058 covers exactly this shape.
func (r *ConsultationRepository) HeldSlots(ctx context.Context, from, to time.Time) ([]time.Time, error) {
	const q = `
		SELECT scheduled_at
		FROM consultations
		WHERE status = ANY(@statuses) AND scheduled_at >= @from AND scheduled_at < @to`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{
		"statuses": consultations.StatusesHoldingSlot,
		"from":     from,
		"to":       to,
	})
	if err != nil {
		return nil, fmt.Errorf("held slots: %w", err)
	}
	defer rows.Close()

	var held []time.Time
	for rows.Next() {
		var slot time.Time
		if err := rows.Scan(&slot); err != nil {
			return nil, fmt.Errorf("held slots: scan: %w", err)
		}
		held = append(held, slot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("held slots: %w", err)
	}
	return held, nil
}

// HoldSlot books the slot a client picked, as a request rather than a booking:
// the price and the confirmation stay with the firm.
//
// The NOT EXISTS is not decoration. Between the picker drawing a grid and the
// client tapping a slot there is a whole round trip, and this closes that
// window down to the width of one statement. It is not a substitute for a
// unique index — two transactions in READ COMMITTED can still both find the
// slot empty — but making the index unique would also forbid the firm from
// ever double-booking an hour deliberately, which is a decision about how they
// work rather than a bug to patch here.
func (r *ConsultationRepository) HoldSlot(ctx context.Context, clientID string, at time.Time, createdBy string) (consultations.Consultation, error) {
	const q = `
		INSERT INTO consultations (client_id, scheduled_at, price, status, created_by)
		SELECT @client_id, @at, 0, @requested, @created_by
		WHERE NOT EXISTS (
			SELECT 1 FROM consultations
			WHERE scheduled_at = @at AND status = ANY(@statuses)
		)
		RETURNING id, client_id, scheduled_at, price, case_note, created_by, status`

	var c consultations.Consultation
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"client_id":  clientID,
		"at":         at,
		"requested":  consultations.StatusRequested,
		"created_by": createdBy,
		"statuses":   consultations.StatusesHoldingSlot,
	}).Scan(&c.ID, &c.ClientID, &c.ScheduledAt, &c.Price, &c.CaseNote, &c.CreatedBy, &c.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return consultations.Consultation{}, consultations.ErrSlotTaken
	}
	if err != nil {
		return consultations.Consultation{}, fmt.Errorf("hold slot for client %q: %w", clientID, err)
	}
	return c, nil
}

// ConsultationsOf lists a client's own consultations, newest appointment first.
func (r *ConsultationRepository) ConsultationsOf(ctx context.Context, clientID string) ([]consultations.Consultation, error) {
	const q = `SELECT id, client_id, scheduled_at, price, case_note, created_by, status
		FROM consultations
		WHERE client_id = @client_id
		ORDER BY scheduled_at DESC`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"client_id": clientID})
	if err != nil {
		return nil, fmt.Errorf("consultations of client %q: %w", clientID, err)
	}
	defer rows.Close()

	var out []consultations.Consultation
	for rows.Next() {
		var c consultations.Consultation
		if err := rows.Scan(&c.ID, &c.ClientID, &c.ScheduledAt, &c.Price, &c.CaseNote, &c.CreatedBy, &c.Status); err != nil {
			return nil, fmt.Errorf("consultations of client %q: scan: %w", clientID, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("consultations of client %q: %w", clientID, err)
	}
	return out, nil
}
