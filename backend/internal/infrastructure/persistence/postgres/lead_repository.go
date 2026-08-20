package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/consultations"
	"multiagent-seo/internal/domain/webleads"
)

type LeadRepository struct {
	db *pgxpool.Pool
}

func NewLeadRepository(db *pgxpool.Pool) *LeadRepository {
	return &LeadRepository{db: db}
}

func (r *LeadRepository) ResolveClient(ctx context.Context, phone, name string) (string, error) {
	if phone == "" {
		return "", nil
	}

	// last_name/first_name/patronymic are the web CRM client card's
	// structured fields — a lead's name arrives as one free-text string,
	// so it's split once here instead of leaving those columns blank
	// forever (see SplitName). Only set on a fresh INSERT: the ON CONFLICT
	// branch never touches them, so a manual correction made later on the
	// client card is never overwritten by a repeat lead from the same phone.
	lastName, firstName, patronymic := consultations.SplitName(name)

	// The ON CONFLICT target's WHERE clause has to repeat the partial unique
	// index's predicate verbatim (see migration 000040) — Postgres only
	// matches ON CONFLICT against a partial index when the inference clause
	// matches its WHERE exactly; without this, every lead with a phone
	// fails to resolve a client at all.
	const q = `INSERT INTO clients (phone, name, last_name, first_name, patronymic)
		VALUES (@phone, @name, @last_name, @first_name, @patronymic)
		ON CONFLICT (phone) WHERE phone IS NOT NULL AND phone <> '' DO UPDATE SET
			last_seen_at = now(),
			name = CASE WHEN @name = '' THEN clients.name ELSE @name END
		RETURNING id`

	var id string
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"phone": phone, "name": name,
		"last_name": lastName, "first_name": firstName, "patronymic": patronymic,
	}).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("resolve client %q: %w", phone, err)
	}
	return id, nil
}

func (r *LeadRepository) Save(ctx context.Context, lead webleads.Lead) error {
	const q = `INSERT INTO leads (
			message_id, received_at, from_email, subject,
			name, phone, message, page, raw_body, telegram_sent_at, client_id, telegram_username
		)
		VALUES (
			@message_id, @received_at, @from_email, @subject,
			@name, @phone, @message, @page, @raw_body, now(), @client_id, @telegram_username
		)
		ON CONFLICT (message_id) DO NOTHING`

	var clientID *string
	if lead.ClientID != "" {
		clientID = &lead.ClientID
	}

	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"message_id":        lead.MessageID,
		"received_at":       lead.ReceivedAt,
		"from_email":        lead.FromEmail,
		"subject":           lead.Subject,
		"name":              lead.Name,
		"phone":             lead.Phone,
		"message":           lead.Message,
		"page":              lead.Page,
		"raw_body":          lead.RawBody,
		"client_id":         clientID,
		"telegram_username": lead.TelegramUsername,
	}); err != nil {
		return fmt.Errorf("save lead %q: %w", lead.MessageID, err)
	}
	return nil
}

func (r *LeadRepository) MarkSheetSynced(ctx context.Context, messageID string) error {
	const q = `UPDATE leads SET sheet_synced_at = now() WHERE message_id = @message_id`
	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{"message_id": messageID}); err != nil {
		return fmt.Errorf("mark sheet synced %q: %w", messageID, err)
	}
	return nil
}
