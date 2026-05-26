CREATE TABLE IF NOT EXISTS articles (
    id          BIGSERIAL PRIMARY KEY,
    keyword     TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    wp_post_id  BIGINT,
    wp_edit_url TEXT,
    wp_post_url TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
