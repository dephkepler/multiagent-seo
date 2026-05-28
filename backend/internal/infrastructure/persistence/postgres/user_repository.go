package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"contentflow/internal/domain/user"
)

var _ user.Repository = (*UserRepository)(nil)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (user.User, error) {
	const q = `
		SELECT id, email, password_hash, created_at, updated_at
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

func scanUser(row pgx.Row) (user.User, error) {
	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}
