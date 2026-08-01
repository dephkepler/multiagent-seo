DROP INDEX IF EXISTS backlink_placements_donor_target_idx;

ALTER TABLE backlink_placements DROP COLUMN IF EXISTS outcome;
