ALTER TABLE articles
    DROP COLUMN IF EXISTS competitor_data,
    DROP COLUMN IF EXISTS check_result;
