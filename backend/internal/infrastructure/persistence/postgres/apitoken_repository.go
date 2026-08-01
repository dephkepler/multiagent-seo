package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/apitoken"
)

type ApiTokenRepository struct {
	db *pgxpool.Pool
}

func NewApiTokenRepository(db *pgxpool.Pool) *ApiTokenRepository {
	return &ApiTokenRepository{db: db}
}

func (r *ApiTokenRepository) Create(ctx context.Context, t apitoken.NewToken) (apitoken.Token, error) {
	const q = `
		INSERT INTO api_tokens (user_id, name, token_hash, prefix)
		VALUES (@user, @name, @hash, @prefix)
		RETURNING id, user_id, name, prefix, created_at, revoked_at`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"user":   t.UserID,
		"name":   t.Name,
		"hash":   t.Hash,
		"prefix": t.Prefix,
	})
	tok, err := scanToken(row)
	if err != nil {
		return apitoken.Token{}, fmt.Errorf("create api token: %w", err)
	}
	return tok, nil
}

func (r *ApiTokenRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]apitoken.Token, error) {
	const q = `
		SELECT id, user_id, name, prefix, created_at, revoked_at
		FROM api_tokens
		WHERE user_id = @user AND revoked_at IS NULL
		ORDER BY created_at`

	rows, err := r.db.Query(ctx, q, pgx.NamedArgs{"user": userID})
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	var out []apitoken.Token
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	return out, nil
}

func (r *ApiTokenRepository) UserByHash(ctx context.Context, hash string) (uuid.UUID, error) {
	const q = `SELECT user_id FROM api_tokens WHERE token_hash = @hash AND revoked_at IS NULL`

	var userID uuid.UUID
	if err := r.db.QueryRow(ctx, q, pgx.NamedArgs{"hash": hash}).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, apitoken.ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("find api token by hash: %w", err)
	}
	return userID, nil
}

func (r *ApiTokenRepository) Revoke(ctx context.Context, userID, id uuid.UUID) error {
	const q = `UPDATE api_tokens SET revoked_at = now() WHERE id = @id AND user_id = @user AND revoked_at IS NULL`

	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id, "user": userID})
	if err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apitoken.ErrNotFound
	}
	return nil
}

func scanToken(row pgx.Row) (apitoken.Token, error) {
	var t apitoken.Token
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.CreatedAt, &t.RevokedAt)
	return t, err
}
