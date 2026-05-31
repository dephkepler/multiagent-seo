ALTER TABLE articles
    ADD COLUMN IF NOT EXISTS competitor_data JSONB,
    ADD COLUMN IF NOT EXISTS check_result    JSONB;
