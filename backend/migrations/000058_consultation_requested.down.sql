-- Fails while any consultation is still 'requested', which is the point: the
-- rows would otherwise violate the narrower constraint being restored.
DROP INDEX IF EXISTS idx_consultations_held_slots;

ALTER TABLE consultations DROP CONSTRAINT IF EXISTS consultations_status_check;

ALTER TABLE consultations ADD CONSTRAINT consultations_status_check
	CHECK (status IN ('scheduled', 'completed', 'cancelled', 'no_show'));
