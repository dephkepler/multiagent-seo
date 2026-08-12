package postgres

import (
	"context"
	"encoding/json"
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
	langs, err := marshalLanguages(in.Languages)
	if err != nil {
		return wordpress.Site{}, fmt.Errorf("marshal languages: %w", err)
	}

	const q = `
		INSERT INTO wordpress_sites (alias, provider, url, username, app_password, languages)
		VALUES (
			@alias, @provider, @url, @username,
			CASE WHEN @app_password = '' THEN NULL ELSE pgp_sym_encrypt(@app_password, @enc_key) END,
			@languages
		)
		RETURNING id, alias, provider, url, coalesce(username, ''), languages, enabled, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"alias":        in.Alias,
		"provider":     string(in.Provider),
		"url":          in.URL,
		"username":     nullify(in.Username),
		"app_password": in.AppPassword,
		"enc_key":      r.encKey,
		"languages":    langs,
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

// nullify turns an empty string into a nil driver value, so optional
// provider-specific fields (e.g. WordPress username on a MODX row) land as
// SQL NULL instead of an empty string.
func nullify(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func marshalLanguages(langs map[string]wordpress.LanguageConfig) ([]byte, error) {
	if langs == nil {
		langs = map[string]wordpress.LanguageConfig{}
	}
	return json.Marshal(langs)
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
		SELECT id, alias, provider, url, coalesce(username, ''), languages, enabled, created_at, updated_at
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
		SELECT id, alias, provider, url, coalesce(username, ''), languages, enabled, created_at, updated_at
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
	var langs []byte
	if in.Languages != nil {
		var err error
		langs, err = json.Marshal(in.Languages)
		if err != nil {
			return wordpress.Site{}, fmt.Errorf("marshal languages: %w", err)
		}
	}

	const q = `
		UPDATE wordpress_sites SET
			alias        = COALESCE(@alias, alias),
			url          = COALESCE(@url, url),
			username     = COALESCE(@username, username),
			enabled      = COALESCE(@enabled, enabled),
			app_password = CASE WHEN @app_password::text IS NULL THEN app_password
			                    ELSE pgp_sym_encrypt(@app_password, @enc_key) END,
			languages    = COALESCE(@languages, languages),
			updated_at   = now()
		WHERE id = @id AND deleted_at IS NULL
		RETURNING id, alias, provider, url, coalesce(username, ''), languages, enabled, created_at, updated_at`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{
		"id":           id,
		"alias":        in.Alias,
		"url":          in.URL,
		"username":     in.Username,
		"enabled":      in.Enabled,
		"app_password": in.AppPassword,
		"enc_key":      r.encKey,
		"languages":    langs,
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
		SELECT provider, url, coalesce(username, ''), coalesce(pgp_sym_decrypt(app_password, @enc_key), ''), languages
		FROM wordpress_sites
		WHERE id = @id AND deleted_at IS NULL AND enabled = true`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{"id": id, "enc_key": r.encKey})
	var (
		c          wordpress.Credentials
		provider   string
		languagesB []byte
	)
	if err := row.Scan(&provider, &c.URL, &c.Username, &c.AppPassword, &languagesB); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wordpress.Credentials{}, wordpress.ErrNotFound
		}
		return wordpress.Credentials{}, fmt.Errorf("get wordpress credentials: %w", err)
	}
	c.Provider = wordpress.Provider(provider)
	langs, err := unmarshalLanguages(languagesB)
	if err != nil {
		return wordpress.Credentials{}, fmt.Errorf("decode languages: %w", err)
	}
	c.Languages = langs
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
	var (
		s          wordpress.Site
		provider   string
		languagesB []byte
	)
	if err := row.Scan(&s.ID, &s.Alias, &provider, &s.URL, &s.Username, &languagesB,
		&s.Enabled, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return wordpress.Site{}, err
	}
	s.Provider = wordpress.Provider(provider)
	langs, err := unmarshalLanguages(languagesB)
	if err != nil {
		return wordpress.Site{}, fmt.Errorf("decode languages: %w", err)
	}
	s.Languages = langs
	return s, nil
}

func unmarshalLanguages(b []byte) (map[string]wordpress.LanguageConfig, error) {
	langs := map[string]wordpress.LanguageConfig{}
	if len(b) == 0 {
		return langs, nil
	}
	if err := json.Unmarshal(b, &langs); err != nil {
		return nil, err
	}
	return langs, nil
}
