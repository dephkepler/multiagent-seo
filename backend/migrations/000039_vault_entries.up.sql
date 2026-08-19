-- Password vault for staff — plain-text storage, deliberately no encryption
-- yet (see doc/standards, revisit before this holds anything more sensitive
-- than internal tool logins). Gated the same way every other admin page is:
-- any authenticated user, there's no separate admin role in this app.
CREATE TABLE vault_entries (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title      text        NOT NULL,
    url        text        NOT NULL DEFAULT '',
    username   text        NOT NULL DEFAULT '',
    password   text        NOT NULL,
    notes      text        NOT NULL DEFAULT '',
    created_by text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
