ALTER TABLE backlink_placements ADD COLUMN IF NOT EXISTS outcome TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS backlink_placements_donor_target_idx
    ON backlink_placements (donor_url, target_url, created_at DESC);
