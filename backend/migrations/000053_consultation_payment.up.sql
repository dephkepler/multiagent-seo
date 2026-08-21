-- A consultation marked «Провів» produced revenue whether or not the client
-- actually paid: the P&L recognises income on status='completed' AND price>0.
-- So consultations were accrual-basis while cases were cash-basis, silently, in
-- the same report — and there was no way to see money a client owed for a
-- consultation that did happen.
--
-- NULL is deliberate and load-bearing: it means "nobody was asked", which is
-- true of every one of the historical rows imported from the spreadsheet. Those
-- keep counting as revenue (that is what the company's own totals did, and they
-- reconcile), while a row where staff answered "не оплачено" does not. Rewriting
-- history into a false certainty would be worse than the gap it closes.
ALTER TABLE consultations ADD COLUMN IF NOT EXISTS paid boolean;

-- The date money changed hands, when it is known. Set to the consultation's own
-- day: the offer requires payment at or before the consultation, so that is the
-- best available answer, and asking staff for a date on every tap is friction
-- they would route around.
ALTER TABLE consultations ADD COLUMN IF NOT EXISTS paid_at date;
