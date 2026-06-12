ALTER TABLE articles
    DROP COLUMN IF EXISTS request_params,
    DROP COLUMN IF EXISTS published_at;
