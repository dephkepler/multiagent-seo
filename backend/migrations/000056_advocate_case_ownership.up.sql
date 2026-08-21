-- Case ownership was never filled in: of 18 cases only 3 carry advocate_id, the
-- rest only a free-text surname. Scoping an advocate's view on advocate_id would
-- therefore show them 3 of their 16 cases, and /finance/settlement attributes
-- 64 000 ₴ of collections to nobody.
--
-- A one-off data repair with the names spelled out, like 000049 and 000050 — not
-- a general mechanism. The names come from this database: "Борзов" (13 cases) and
-- "Ярослав Борзов" (3) are the same person entered twice, and only the second is
-- on the roster; "Бойчук" and "Попов" each hold one case and are missing from it.

-- Put the two missing advocates on the roster — but only where there is
-- actually a case naming them. A fresh database (dev, tests, a future install)
-- has no such cases and must not end up with two invented advocates because a
-- repair migration ran. commission_percent stays 0: the real rate is a business
-- decision, and inventing one would generate payouts.
INSERT INTO advocates (full_name, is_active, commission_percent)
SELECT DISTINCT c.advocate_name, true, 0
FROM cases c
WHERE c.advocate_id IS NULL
  AND c.advocate_name IN ('Бойчук', 'Попов')
  AND NOT EXISTS (SELECT 1 FROM advocates a WHERE a.full_name = c.advocate_name);

-- Merge the duplicate spelling: every case written as "Борзов" belongs to the
-- roster row "Ярослав Борзов".
UPDATE cases c
SET advocate_id = (SELECT id FROM advocates WHERE full_name = 'Ярослав Борзов' LIMIT 1)
WHERE c.advocate_id IS NULL
  AND c.advocate_name IN ('Борзов', 'Ярослав Борзов')
  AND EXISTS (SELECT 1 FROM advocates WHERE full_name = 'Ярослав Борзов');

-- Everything else links by an exact name match. Rows that match nobody are left
-- alone on purpose: they surface in /finance/gaps as unlinked_cases, which is
-- honest, where a guess would not be.
UPDATE cases c
SET advocate_id = a.id
FROM advocates a
WHERE c.advocate_id IS NULL AND c.advocate_name = a.full_name;
