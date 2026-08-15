-- Advocates stop being a single overwritten slot and become a real roster:
-- /advocate now inserts a new row instead of updating the only one. Leaving
-- a firm doesn't delete the row (cases/history stay intact) — it just flips
-- is_active so the advocate drops out of pickers for new work.
ALTER TABLE advocates ADD COLUMN IF NOT EXISTS is_active boolean NOT NULL DEFAULT true;

-- Which advocate is actually handling a case — was free-text advocate_name
-- (still kept for historical/imported cases that predate the roster), now a
-- real link for cases created through the picker.
ALTER TABLE cases ADD COLUMN IF NOT EXISTS advocate_id uuid REFERENCES advocates(id);
