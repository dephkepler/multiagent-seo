CREATE TABLE IF NOT EXISTS prompt_outcomes (
    id              BIGSERIAL PRIMARY KEY,
    article_id      BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    stage           TEXT NOT NULL,
    variant_id      BIGINT REFERENCES prompt_variants(id) ON DELETE SET NULL,
    reward          REAL,
    ai_score        REAL,
    quality_ok      BOOLEAN NOT NULL DEFAULT FALSE,
    humanize_cycles INTEGER NOT NULL DEFAULT 0,
    tokens          INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_prompt_outcomes_variant ON prompt_outcomes (variant_id, created_at);
