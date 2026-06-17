CREATE TABLE scrape_jobs (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    sheet      text        NOT NULL,
    state      text        NOT NULL DEFAULT 'running',
    total      integer     NOT NULL DEFAULT 0,
    processed  integer     NOT NULL DEFAULT 0,
    error      text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_scrape_jobs_state CHECK (state IN ('running', 'done', 'canceled', 'failed'))
);
