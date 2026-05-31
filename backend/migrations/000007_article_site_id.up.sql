ALTER TABLE articles
    ADD COLUMN IF NOT EXISTS site_id uuid;

-- The code only ever writes 'generating' on insert; 'pending' was never used.
ALTER TABLE articles
    ALTER COLUMN status SET DEFAULT 'generating';
