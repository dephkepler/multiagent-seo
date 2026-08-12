ALTER TABLE wordpress_sites
    DROP COLUMN languages;

ALTER TABLE wordpress_sites
    ADD COLUMN db_host      text,
    ADD COLUMN db_port      text,
    ADD COLUMN db_password  bytea,
    ADD COLUMN db_name      text,
    ADD COLUMN context_key  text,
    ADD COLUMN keywords_spreadsheet_id text,
    ADD COLUMN keywords_sheet          text;
