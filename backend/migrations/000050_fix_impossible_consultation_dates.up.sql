-- One consultation came out of the historical import dated 2002-10-11 while the
-- row itself was created 2024-10-11: a year parsed wrong at the source. It is
-- 800 ₴ of completed revenue that falls outside every sane report window, and
-- it dragged the finance page's "all time" period back twenty-two years, which
-- is how it was noticed.
--
-- created_at is the only other date these rows carry, and for imported rows it
-- is the date the money actually happened (the importer set it from the sheet).
-- The 2020 floor is a sanity bound, not a business rule: the firm did not exist
-- before it, so anything earlier is an artifact rather than history.
UPDATE consultations
SET scheduled_at = created_at
WHERE scheduled_at < timestamptz '2020-01-01'
  AND created_at >= timestamptz '2020-01-01';
