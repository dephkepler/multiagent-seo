ALTER TABLE consultations ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'scheduled';

ALTER TABLE consultations ADD CONSTRAINT consultations_status_check
	CHECK (status IN ('scheduled', 'completed', 'cancelled', 'no_show'));
