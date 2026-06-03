CREATE TABLE donor_site_credentials (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    donor_url    text        NOT NULL,
    login        text        NOT NULL,
    app_password bytea       NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);

-- One credential per donor URL among live rows; a soft-deleted URL can be reused.
CREATE UNIQUE INDEX uq_donor_site_credentials_url_live
    ON donor_site_credentials (donor_url)
    WHERE deleted_at IS NULL;
