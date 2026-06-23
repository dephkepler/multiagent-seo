CREATE TABLE IF NOT EXISTS backlink_placements (
    id         BIGSERIAL PRIMARY KEY,
    run_id     TEXT NOT NULL,
    sheet      TEXT NOT NULL,
    donor_url  TEXT NOT NULL,
    target_url TEXT NOT NULL,
    ok         BOOLEAN NOT NULL,
    status     TEXT NOT NULL,
    post_url   TEXT NOT NULL DEFAULT '',
    edit_url   TEXT NOT NULL DEFAULT '',
    anchor     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS backlink_placements_run_idx ON backlink_placements (run_id, created_at);
