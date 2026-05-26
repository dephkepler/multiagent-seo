ALTER TABLE articles
    DROP COLUMN IF EXISTS images_requested,
    DROP COLUMN IF EXISTS images_resolved,
    DROP COLUMN IF EXISTS images_skipped;
