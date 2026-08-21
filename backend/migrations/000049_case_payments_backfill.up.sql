-- Cases imported before the case_payments ledger existed (000041) carry their
-- money as a bare running total in cases.paid_amount, with no ledger row behind
-- it. That was invisible while only leadstats read this data — it sums the
-- column — but the finance report sums the ledger, so the same collected money
-- reads as 100 000 on /leads and 0 on /finance.
--
-- One row per such case makes the ledger the single source of truth again.
-- paid_at is the case's own created_at: the real payment dates were never
-- recorded, and spreading them across months would put money in months it may
-- not belong to. created_by marks these rows so the down migration undoes
-- exactly this insert and nothing a human typed.
INSERT INTO case_payments (case_id, amount, paid_at, created_by)
SELECT c.id, c.paid_amount, c.created_at::date, 'backfill'
FROM cases c
WHERE c.paid_amount > 0
  AND NOT EXISTS (SELECT 1 FROM case_payments p WHERE p.case_id = c.id);
