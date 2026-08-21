-- A client picking a slot in the Mini App writes the consultation themselves,
-- so there is now a state before 'scheduled': the slot is held, but the price
-- and the confirmation are still the firm's to give.
ALTER TABLE consultations DROP CONSTRAINT IF EXISTS consultations_status_check;

ALTER TABLE consultations ADD CONSTRAINT consultations_status_check
	CHECK (status IN ('requested', 'scheduled', 'completed', 'cancelled', 'no_show'));

-- The slot picker asks "what is taken between these two instants" on every
-- launch, and consultations carried only an index on client_id (000024).
-- Partial, because a completed or cancelled consultation holds nothing.
CREATE INDEX IF NOT EXISTS idx_consultations_held_slots
	ON consultations (scheduled_at)
	WHERE status IN ('requested', 'scheduled');
