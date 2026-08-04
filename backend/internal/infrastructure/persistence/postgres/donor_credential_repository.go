package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"multiagent-seo/internal/domain/linkbuilding"
)

type DonorCredentialRepository struct {
	db     *pgxpool.Pool
	encKey string
}

func NewDonorCredentialRepository(db *pgxpool.Pool, encKey string) *DonorCredentialRepository {
	return &DonorCredentialRepository{db: db, encKey: encKey}
}

func (r *DonorCredentialRepository) Get(ctx context.Context, donorURL string) (linkbuilding.DonorCredential, bool, error) {
	const q = `
		SELECT donor_url, login, pgp_sym_decrypt(app_password, @enc_key)
		FROM donor_site_credentials
		WHERE donor_url = @donor_url AND deleted_at IS NULL`

	row := r.db.QueryRow(ctx, q, pgx.NamedArgs{"donor_url": donorURL, "enc_key": r.encKey})
	var c linkbuilding.DonorCredential
	if err := row.Scan(&c.DonorURL, &c.Login, &c.AppPassword); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return linkbuilding.DonorCredential{}, false, nil
		}
		return linkbuilding.DonorCredential{}, false, fmt.Errorf("get donor credential: %w", err)
	}
	return c, true, nil
}

func (r *DonorCredentialRepository) Save(ctx context.Context, cred linkbuilding.DonorCredential) error {
	const q = `
		INSERT INTO donor_site_credentials (donor_url, login, app_password)
		VALUES (@donor_url, @login, pgp_sym_encrypt(@app_password, @enc_key))
		ON CONFLICT (donor_url) WHERE deleted_at IS NULL DO UPDATE SET
			login        = EXCLUDED.login,
			app_password = pgp_sym_encrypt(@app_password, @enc_key),
			updated_at   = now()`

	if _, err := r.db.Exec(ctx, q, pgx.NamedArgs{
		"donor_url":    cred.DonorURL,
		"login":        cred.Login,
		"app_password": cred.AppPassword,
		"enc_key":      r.encKey,
	}); err != nil {
		return fmt.Errorf("save donor credential: %w", err)
	}
	return nil
}
