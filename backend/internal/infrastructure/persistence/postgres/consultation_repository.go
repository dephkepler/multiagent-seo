package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/consultations"
)

type ConsultationRepository struct {
	db *pgxpool.Pool
}

func NewConsultationRepository(db *pgxpool.Pool) *ConsultationRepository {
	return &ConsultationRepository{db: db}
}

func (r *ConsultationRepository) FindClient(ctx context.Context, clientID string) (consultations.Client, error) {
	const q = `SELECT id, name, phone, coalesce(telegram_name, '') FROM clients WHERE id = @id`
	var c consultations.Client
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": clientID}).Scan(&c.ID, &c.Name, &c.Phone, &c.TelegramName)
	if err != nil {
		return consultations.Client{}, fmt.Errorf("find client %q: %w", clientID, err)
	}
	return c, nil
}

func (r *ConsultationRepository) SearchClients(ctx context.Context, query string) ([]consultations.Client, error) {
	const q = `SELECT id, name, phone, coalesce(telegram_name, '')
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

func (r *ConsultationRepository) SetClientTelegram(ctx context.Context, clientID string, chatID int64, telegramName string) error {
	const q = `UPDATE clients SET telegram_chat_id = @chat_id, telegram_name = @telegram_name WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"chat_id": chatID, "telegram_name": telegramName, "id": clientID})
	if err != nil {
		return fmt.Errorf("set telegram info for client %q: %w", clientID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set telegram info: no client with id %q", clientID)
	}
	return nil
}

func (r *ConsultationRepository) UpdateClient(ctx context.Context, clientID string, edit consultations.ClientEdit) error {
	const q = `UPDATE clients SET
			name = @name, last_name = @last_name, first_name = @first_name, patronymic = @patronymic, phone = @phone
		WHERE id = @id`
	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"name":       consultations.ComposeName(edit.LastName, edit.FirstName, edit.Patronymic),
		"last_name":  edit.LastName,
		"first_name": edit.FirstName,
		"patronymic": edit.Patronymic,
		"phone":      edit.Phone,
		"id":         clientID,
	})
	if err != nil {
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
			cl.id, cl.name, cl.phone, coalesce(cl.telegram_name, ''), coalesce(cl.telegram_chat_id, 0)
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
			cl.id, cl.name, cl.phone, coalesce(cl.telegram_name, ''), coalesce(cl.telegram_chat_id, 0)
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
