package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/vault"
)

type VaultRepository struct {
	db *pgxpool.Pool
}

func NewVaultRepository(db *pgxpool.Pool) *VaultRepository {
	return &VaultRepository{db: db}
}

func (r *VaultRepository) Create(ctx context.Context, in vault.CreateEntry) (vault.Entry, error) {
	const q = `
		INSERT INTO vault_entries (title, url, username, password, notes, created_by)
		VALUES (@title, @url, @username, @password, @notes, @created_by)
		RETURNING id, title, url, username, password, notes, created_by, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"title":      in.Title,
		"url":        in.URL,
		"username":   in.Username,
		"password":   in.Password,
		"notes":      in.Notes,
		"created_by": in.CreatedBy,
	})
	entry, err := scanVaultEntry(row)
	if err != nil {
		return vault.Entry{}, fmt.Errorf("create vault entry: %w", err)
	}
	return entry, nil
}

func (r *VaultRepository) List(ctx context.Context) ([]vault.Entry, error) {
	const q = `
		SELECT id, title, url, username, password, notes, created_by, created_at, updated_at
		FROM vault_entries
		ORDER BY title`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list vault entries: %w", err)
	}
	defer rows.Close()

	entries := make([]vault.Entry, 0)
	for rows.Next() {
		entry, err := scanVaultEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan vault entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list vault entries: %w", err)
	}
	return entries, nil
}

func (r *VaultRepository) Update(ctx context.Context, id uuid.UUID, in vault.UpdateEntry) (vault.Entry, error) {
	const q = `
		UPDATE vault_entries SET
			title      = COALESCE(@title, title),
			url        = COALESCE(@url, url),
			username   = COALESCE(@username, username),
			password   = COALESCE(@password, password),
			notes      = COALESCE(@notes, notes),
			updated_at = now()
		WHERE id = @id
		RETURNING id, title, url, username, password, notes, created_by, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"id":       id,
		"title":    in.Title,
		"url":      in.URL,
		"username": in.Username,
		"password": in.Password,
		"notes":    in.Notes,
	})
	entry, err := scanVaultEntry(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return vault.Entry{}, vault.ErrNotFound
		}
		return vault.Entry{}, fmt.Errorf("update vault entry: %w", err)
	}
	return entry, nil
}

func (r *VaultRepository) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM vault_entries WHERE id = @id`

	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id})
	if err != nil {
		return fmt.Errorf("delete vault entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return vault.ErrNotFound
	}
	return nil
}

func scanVaultEntry(row pgx.Row) (vault.Entry, error) {
	var e vault.Entry
	if err := row.Scan(&e.ID, &e.Title, &e.URL, &e.Username, &e.Password, &e.Notes,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return vault.Entry{}, err
	}
	return e, nil
}
