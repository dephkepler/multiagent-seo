CREATE TABLE IF NOT EXISTS article_revisions (
    id              BIGSERIAL PRIMARY KEY,
    article_id      BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    source          TEXT NOT NULL,
    content_md      TEXT NOT NULL,
    content_html    TEXT NOT NULL,
    seo_title       TEXT,
    seo_description TEXT,
    word_count      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_article_revisions_article ON article_revisions (article_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_article_revisions_version ON article_revisions (article_id, version);
