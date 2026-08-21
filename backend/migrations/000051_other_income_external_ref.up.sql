-- other_income needs an idempotency key for the same reason expenses have one:
-- the top-up rows written to close the gap between what the CRM can see and
-- what the company's own monthly totals say must be re-runnable. Without a key,
-- running the reconciliation twice would book January's missing 46 800 ₴ twice.
--
-- Partial unique index, and the predicate matters: hand-entered other-income
-- rows carry no ref, and any number of them must be able to coexist.
ALTER TABLE other_income ADD COLUMN IF NOT EXISTS external_ref text;

CREATE UNIQUE INDEX IF NOT EXISTS uq_other_income_external_ref
    ON other_income (external_ref) WHERE external_ref IS NOT NULL;
