package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/user"
)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (user.User, error) {
	const q = `
		SELECT id, email, password_hash, role, coalesce(advocate_id::text, ''), created_at, updated_at
		FROM users
		WHERE email = @email`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{"email": email})
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return user.User{}, user.ErrNotFound
		}
		return user.User{}, fmt.Errorf("find user by email: %w", err)
	}
	return u, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (user.User, error) {
	const q = `
		SELECT id, email, password_hash, role, coalesce(advocate_id::text, ''), created_at, updated_at
		FROM users
		WHERE id = @id`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": id})
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return user.User{}, user.ErrNotFound
		}
		return user.User{}, fmt.Errorf("find user by id: %w", err)
	}
	return u, nil
}

func (r *UserRepository) List(ctx context.Context) ([]user.User, error) {
	const q = `SELECT id, email, password_hash, role, coalesce(advocate_id::text, ''), created_at, updated_at FROM users ORDER BY created_at`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var out []user.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return out, nil
}

func scanUser(row pgx.Row) (user.User, error) {
	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.AdvocateID, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}
