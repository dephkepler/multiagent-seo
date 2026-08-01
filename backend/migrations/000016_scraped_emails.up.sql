CREATE TABLE scraped_emails (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    list_name   text        NOT NULL,
    site_url    text        NOT NULL,
    email       text        NOT NULL,
    source_page text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_scraped_emails UNIQUE (list_name, site_url, email)
);
