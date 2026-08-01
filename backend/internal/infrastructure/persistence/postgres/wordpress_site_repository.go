package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/wordpress"
)

type WordpressSiteRepository struct {
	db     *pgxpool.Pool
	encKey string
}

func NewWordpressSiteRepository(db *pgxpool.Pool, encKey string) *WordpressSiteRepository {
	return &WordpressSiteRepository{db: db, encKey: encKey}
}

func (r *WordpressSiteRepository) Create(ctx context.Context, in wordpress.CreateSite) (wordpress.Site, error) {
	const q = `
		INSERT INTO wordpress_sites (alias, url, username, app_password)
		VALUES (@alias, @url, @username, pgp_sym_encrypt(@app_password, @enc_key))
		RETURNING id, alias, url, username, enabled, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"alias":        in.Alias,
		"url":          in.URL,
		"username":     in.Username,
		"app_password": in.AppPassword,
		"enc_key":      r.encKey,
	})

	site, err := scanSite(row)
	if err != nil {
		if mapped := mapSiteAliasError(err); mapped != nil {
			return wordpress.Site{}, mapped
		}
		return wordpress.Site{}, fmt.Errorf("create wordpress site: %w", err)
	}
	return site, nil
}

func mapSiteAliasError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return wordpress.ErrAliasExists
	}
	return nil
}

func (r *WordpressSiteRepository) List(ctx context.Context) ([]wordpress.Site, error) {
	const q = `
		SELECT id, alias, url, username, enabled, created_at, updated_at
		FROM wordpress_sites
		WHERE deleted_at IS NULL
		ORDER BY created_at`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list wordpress sites: %w", err)
	}
	defer rows.Close()

	sites := make([]wordpress.Site, 0)
	for rows.Next() {
		site, err := scanSite(rows)
		if err != nil {
			return nil, fmt.Errorf("scan wordpress site: %w", err)
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list wordpress sites: %w", err)
	}
	return sites, nil
}

func (r *WordpressSiteRepository) Get(ctx context.Context, id uuid.UUID) (wordpress.Site, error) {
	const q = `
		SELECT id, alias, url, username, enabled, created_at, updated_at
		FROM wordpress_sites
		WHERE id = @id AND deleted_at IS NULL`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": id})
	site, err := scanSite(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wordpress.Site{}, wordpress.ErrNotFound
		}
		return wordpress.Site{}, fmt.Errorf("get wordpress site: %w", err)
	}
	return site, nil
}

func (r *WordpressSiteRepository) Update(ctx context.Context, id uuid.UUID, in wordpress.UpdateSite) (wordpress.Site, error) {
	const q = `
		UPDATE wordpress_sites SET
			alias        = COALESCE(@alias, alias),
			url          = COALESCE(@url, url),
			username     = COALESCE(@username, username),
			enabled      = COALESCE(@enabled, enabled),
			app_password = CASE WHEN @app_password::text IS NULL THEN app_password
			                    ELSE pgp_sym_encrypt(@app_password, @enc_key) END,
			updated_at   = now()
		WHERE id = @id AND deleted_at IS NULL
		RETURNING id, alias, url, username, enabled, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"id":           id,
		"alias":        in.Alias,
		"url":          in.URL,
		"username":     in.Username,
		"enabled":      in.Enabled,
		"app_password": in.AppPassword,
		"enc_key":      r.encKey,
	})

	site, err := scanSite(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wordpress.Site{}, wordpress.ErrNotFound
		}
		if mapped := mapSiteAliasError(err); mapped != nil {
			return wordpress.Site{}, mapped
		}
		return wordpress.Site{}, fmt.Errorf("update wordpress site: %w", err)
	}
	return site, nil
}

func (r *WordpressSiteRepository) Credentials(ctx context.Context, id uuid.UUID) (wordpress.Credentials, error) {
	const q = `
		SELECT url, username, pgp_sym_decrypt(app_password, @enc_key)
		FROM wordpress_sites
		WHERE id = @id AND deleted_at IS NULL AND enabled = true`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": id, "enc_key": r.encKey})
	var c wordpress.Credentials
	if err := row.Scan(&c.URL, &c.Username, &c.AppPassword); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wordpress.Credentials{}, wordpress.ErrNotFound
		}
		return wordpress.Credentials{}, fmt.Errorf("get wordpress credentials: %w", err)
	}
	return c, nil
}

func (r *WordpressSiteRepository) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `
		UPDATE wordpress_sites SET deleted_at = now()
		WHERE id = @id AND deleted_at IS NULL`

	tag, err := r.db.Exec(ctx, q, pgx.NamedArgs{"id": id})
	if err != nil {
		return fmt.Errorf("delete wordpress site: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return wordpress.ErrNotFound
	}
	return nil
}

func scanSite(row pgx.Row) (wordpress.Site, error) {
	var s wordpress.Site
	err := row.Scan(&s.ID, &s.Alias, &s.URL, &s.Username, &s.Enabled, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}
