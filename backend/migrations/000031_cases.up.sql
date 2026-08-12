CREATE TABLE IF NOT EXISTS cases (
	id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
	client_id       uuid        NOT NULL REFERENCES clients(id),
	consultation_id uuid        REFERENCES consultations(id),
	advocate_name   text        NOT NULL DEFAULT '',
	fee             numeric     NOT NULL DEFAULT 0,
	paid_amount     numeric     NOT NULL DEFAULT 0,
	status          text        NOT NULL DEFAULT 'in_progress',
	description     text        NOT NULL DEFAULT '',
	created_by      text        NOT NULL DEFAULT '',
	created_at      timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE cases ADD CONSTRAINT cases_status_check
	CHECK (status IN ('in_progress', 'completed', 'cancelled'));

CREATE INDEX IF NOT EXISTS idx_cases_client_id ON cases (client_id);
