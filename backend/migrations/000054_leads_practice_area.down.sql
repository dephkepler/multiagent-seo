DROP INDEX IF EXISTS idx_leads_telegram_message_id;
ALTER TABLE leads DROP COLUMN IF EXISTS telegram_message_id;
ALTER TABLE leads DROP COLUMN IF EXISTS practice_area;
