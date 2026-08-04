package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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

	const q = `INSERT INTO clients (phone, name)
		VALUES (@phone, @name)
		ON CONFLICT (phone) DO UPDATE SET
			last_seen_at = now(),
			name = CASE WHEN @name = '' THEN clients.name ELSE @name END
		RETURNING id`

	var id string
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"phone": phone, "name": name}).Scan(&id)
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
