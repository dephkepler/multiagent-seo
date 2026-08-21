-- practice_area lets staff tag what a lead is about right when it comes
-- in (via buttons on the lead's Telegram notification, see
-- webleads.PracticeAreaButtons) instead of only finding out weeks later
-- when/if it becomes a case (cases.category). Empty for every lead until
-- tagged, and forever for leads predating this feature.
ALTER TABLE leads ADD COLUMN IF NOT EXISTS practice_area text NOT NULL DEFAULT '';

-- telegram_message_id is the id of the Telegram message the lead was
-- announced in — captured right after the notify send succeeds. It's the
-- join key a later button tap uses to record practice_area (see
-- Store.SetPracticeArea): the tap's own message carries this id, so no
-- separate identifier needs to travel in callback_data (Telegram's
-- 64-byte cap makes that risky — see ManualTagMaxLen's history). 0 means
-- "no Telegram message" (shouldn't happen going forward, but leads
-- created before this migration have no value to backfill).
ALTER TABLE leads ADD COLUMN IF NOT EXISTS telegram_message_id integer NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_leads_telegram_message_id ON leads (telegram_message_id) WHERE telegram_message_id <> 0;
