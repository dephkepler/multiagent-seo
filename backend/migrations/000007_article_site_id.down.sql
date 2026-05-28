ALTER TABLE articles
    ALTER COLUMN status SET DEFAULT 'pending';

ALTER TABLE articles
    DROP COLUMN IF EXISTS site_id;
