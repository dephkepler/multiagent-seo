-- Manual override for a client's funnel segment. Normally Derive (see
-- clientsegments package) computes the segment purely from leads/
-- consultations/cases — this column is the deliberate escape hatch for
-- when the data doesn't tell the whole story (e.g. a client who moved to
-- another advocate outside this system entirely — see ABL 024). NULL means
-- "no override, use the calculated value."
ALTER TABLE clients ADD COLUMN IF NOT EXISTS segment_override text;

ALTER TABLE clients ADD CONSTRAINT clients_segment_override_check
    CHECK (segment_override IS NULL OR segment_override IN ('lead', 'booked', 'consulted', 'client', 'repeat', 'lost'));
