package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/correspondence"
)

type CorrespondenceRepository struct {
	db *pgxpool.Pool
}

func NewCorrespondenceRepository(db *pgxpool.Pool) *CorrespondenceRepository {
	return &CorrespondenceRepository{db: db}
}

func (r *CorrespondenceRepository) UpsertContact(ctx context.Context, c correspondence.Contact) (correspondence.Contact, error) {
	const q = `INSERT INTO telegram_contacts (telegram_user_id, username, first_name, last_name, phone)
		VALUES (@telegram_user_id, @username, @first_name, @last_name, @phone)
		ON CONFLICT (telegram_user_id) DO UPDATE SET
			username     = @username,
			first_name   = @first_name,
			last_name    = @last_name,
			phone        = CASE WHEN @phone = '' THEN telegram_contacts.phone ELSE @phone END,
			last_seen_at = now()
		RETURNING id, first_seen_at, last_seen_at`

	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"telegram_user_id": c.TelegramUserID,
		"username":         c.Username,
		"first_name":       c.FirstName,
		"last_name":        c.LastName,
		"phone":            c.Phone,
	}).Scan(&c.ID, &c.FirstSeenAt, &c.LastSeenAt)
	if err != nil {
		return correspondence.Contact{}, fmt.Errorf("upsert contact %d: %w", c.TelegramUserID, err)
	}
	return c, nil
}

func (r *CorrespondenceRepository) SaveMessage(ctx context.Context, m correspondence.Message) error {
	const q = `INSERT INTO telegram_messages (contact_id, telegram_message_id, direction, text, sent_at)
		VALUES (@contact_id, @telegram_message_id, @direction, @text, @sent_at)
		ON CONFLICT (contact_id, telegram_message_id) DO NOTHING`

	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"contact_id":          m.ContactID,
		"telegram_message_id": m.TelegramMessageID,
		"direction":           string(m.Direction),
		"text":                m.Text,
		"sent_at":             m.SentAt,
	}); err != nil {
		return fmt.Errorf("save message %d for contact %s: %w", m.TelegramMessageID, m.ContactID, err)
	}
	return nil
}
