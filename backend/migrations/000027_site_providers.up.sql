ALTER TABLE wordpress_sites
    ALTER COLUMN username DROP NOT NULL,
    ALTER COLUMN app_password DROP NOT NULL;

ALTER TABLE wordpress_sites
    ADD COLUMN provider     text NOT NULL DEFAULT 'wordpress',
    ADD COLUMN db_host      text,
    ADD COLUMN db_port      text,
    ADD COLUMN db_password  bytea,
    ADD COLUMN db_name      text,
    ADD COLUMN context_key  text;
