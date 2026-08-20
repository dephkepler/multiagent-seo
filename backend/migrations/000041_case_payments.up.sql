-- cases.paid_amount is (and stays) a running total — this table is the
-- ledger behind it: one row per /pay, so "how much did the client pay and
-- when" is answerable per installment, not just as a single cumulative
-- number with no history. paid_at is a plain date (staff only ever gives a
-- day, never a time) and defaults to today so the existing one-shot
-- "/pay <id> <amount>" syntax (no date argument) keeps working unchanged.
CREATE TABLE IF NOT EXISTS case_payments (
	id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
	case_id    uuid        NOT NULL REFERENCES cases(id),
	amount     numeric     NOT NULL,
	paid_at    date        NOT NULL DEFAULT current_date,
	created_by text        NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_case_payments_case_id ON case_payments (case_id);
