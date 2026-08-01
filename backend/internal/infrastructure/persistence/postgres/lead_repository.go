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

// ResolveClient finds-or-creates the client for phone (same phone across
// leads = same client). Every match, new or returning, bumps last_seen_at
// and refreshes name to whatever this lead gave, since that's the most
// recent info on file.
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

// Save inserts a lead. lead.ClientID is expected to already be resolved
// (via ResolveClient) — Save just persists it, it doesn't re-derive it. A
// duplicate MessageID (e.g. the same message processed twice after a
// crash) is silently ignored rather than erroring.
func (r *LeadRepository) Save(ctx context.Context, lead webleads.Lead) error {
	const q = `INSERT INTO leads (
			message_id, received_at, from_email, subject,
			name, phone, message, page, raw_body, telegram_sent_at, client_id
		)
		VALUES (
			@message_id, @received_at, @from_email, @subject,
			@name, @phone, @message, @page, @raw_body, now(), @client_id
		)
		ON CONFLICT (message_id) DO NOTHING`

	var clientID *string
	if lead.ClientID != "" {
		clientID = &lead.ClientID
	}

	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"message_id":  lead.MessageID,
		"received_at": lead.ReceivedAt,
		"from_email":  lead.FromEmail,
		"subject":     lead.Subject,
		"name":        lead.Name,
		"phone":       lead.Phone,
		"message":     lead.Message,
		"page":        lead.Page,
		"raw_body":    lead.RawBody,
		"client_id":   clientID,
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
