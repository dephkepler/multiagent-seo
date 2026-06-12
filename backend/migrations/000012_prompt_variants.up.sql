CREATE TABLE IF NOT EXISTS prompt_variants (
    id         BIGSERIAL PRIMARY KEY,
    stage      TEXT NOT NULL,
    body       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'candidate',
    origin     TEXT NOT NULL DEFAULT 'seed',
    parent_id  BIGINT REFERENCES prompt_variants(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_prompt_variants_stage_status ON prompt_variants (stage, status);
CREATE UNIQUE INDEX IF NOT EXISTS uq_prompt_variants_champion ON prompt_variants (stage) WHERE status = 'champion';
