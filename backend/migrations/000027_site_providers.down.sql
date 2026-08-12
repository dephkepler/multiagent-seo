ALTER TABLE wordpress_sites
    DROP COLUMN provider,
    DROP COLUMN db_host,
    DROP COLUMN db_port,
    DROP COLUMN db_password,
    DROP COLUMN db_name,
    DROP COLUMN context_key;

ALTER TABLE wordpress_sites
    ALTER COLUMN username SET NOT NULL,
    ALTER COLUMN app_password SET NOT NULL;
