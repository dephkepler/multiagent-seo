CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE wordpress_sites (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    alias        text        NOT NULL,
    url          text        NOT NULL,
    username     text        NOT NULL,
    app_password bytea       NOT NULL,
    enabled      boolean     NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);

-- Alias is unique only among live rows, so a soft-deleted alias can be reused.
CREATE UNIQUE INDEX uq_wordpress_sites_alias_live
    ON wordpress_sites (alias)
    WHERE deleted_at IS NULL;
