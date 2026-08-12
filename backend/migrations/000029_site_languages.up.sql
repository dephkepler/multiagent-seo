ALTER TABLE wordpress_sites
    DROP COLUMN db_host,
    DROP COLUMN db_port,
    DROP COLUMN db_password,
    DROP COLUMN db_name,
    DROP COLUMN context_key,
    DROP COLUMN keywords_spreadsheet_id,
    DROP COLUMN keywords_sheet;

ALTER TABLE wordpress_sites
    ADD COLUMN languages jsonb NOT NULL DEFAULT '{}'::jsonb;
