CREATE TABLE IF NOT EXISTS donor_site_profiles (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    donor_url       text        NOT NULL,
    role            text        NOT NULL DEFAULT '',
    can_edit_pages  boolean     NOT NULL DEFAULT false,
    can_edit_others boolean     NOT NULL DEFAULT false,
    can_publish     boolean     NOT NULL DEFAULT false,
    can_create      boolean     NOT NULL DEFAULT false,
    last_outcome    text        NOT NULL DEFAULT '',
    detected_at     timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_donor_site_profiles_url ON donor_site_profiles (donor_url);
