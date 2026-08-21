package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/user"
)

type TelegramRepository struct {
	db *pgxpool.Pool
}

func NewTelegramRepository(db *pgxpool.Pool) *TelegramRepository {
	return &TelegramRepository{db: db}
}

// FindByTelegramID looks in the roster before the client list, because the two
// are not mutually exclusive: an advocate who was once a client of the firm has
// a row in both, and the roster is the more privileged of the two readings.
//
// Only an active advocate authenticates as one. Someone who left the firm keeps
// their row — cases and consultations still point at it — but stops being a
// caller, and falls through to whatever else the id matches.
func (r *TelegramRepository) FindByTelegramID(ctx context.Context, telegramID int64) (user.TelegramSubject, error) {
	const q = `
		SELECT role, id FROM (
			SELECT 1 AS priority, 'advocate' AS role, id::text AS id
			FROM advocates
			WHERE telegram_chat_id = @tg AND is_active
			UNION ALL
			SELECT 2 AS priority, 'client' AS role, id::text AS id
			FROM clients
			WHERE telegram_chat_id = @tg
		) matches
		ORDER BY priority
		LIMIT 1`

	var role, id string
	err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"tg": telegramID}).Scan(&role, &id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return user.TelegramSubject{}, user.ErrNotFound
		}
		return user.TelegramSubject{}, fmt.Errorf("find subject by telegram id: %w", err)
	}

	if role == "advocate" {
		return user.TelegramSubject{Role: user.RoleAdvocate, AdvocateID: id}, nil
	}
	return user.TelegramSubject{Role: user.RoleClient, ClientID: id}, nil
}
