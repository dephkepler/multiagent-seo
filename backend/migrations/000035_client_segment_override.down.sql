ALTER TABLE clients DROP CONSTRAINT IF EXISTS clients_segment_override_check;
ALTER TABLE clients DROP COLUMN IF EXISTS segment_override;
