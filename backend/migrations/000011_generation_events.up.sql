CREATE TABLE IF NOT EXISTS generation_events (
    id            BIGSERIAL PRIMARY KEY,
    article_id    BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    stage         TEXT NOT NULL,
    provider      TEXT,
    model         TEXT,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms    BIGINT NOT NULL DEFAULT 0,
    ok            BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_generation_events_article ON generation_events (article_id);
CREATE INDEX IF NOT EXISTS idx_generation_events_stage ON generation_events (stage);
