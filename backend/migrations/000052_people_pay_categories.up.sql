-- The P&L classifies spend by what the work was FOR (marketing, development,
-- delivery), which is the right shape for a P&L and the shape the company's own
-- spreadsheet used. The consequence is that money paid to people is scattered
-- across four kinds — the "Зарплаты" line shows only the assistant (56 300 ₴)
-- while 353 950 ₴ of 599 967 ₴, 59% of everything, is people doing work.
--
-- This flag is the second dimension: who the money went to, not what for. It
-- answers "сколько стоят люди" without re-classifying anything, so both views
-- stay available.
--
-- Advocates are included: they are paid for delivered work like everyone else.
-- Ads, hosting, telephony, company fees are not — nobody is paid there.
ALTER TABLE expense_categories ADD COLUMN IF NOT EXISTS is_people_pay boolean NOT NULL DEFAULT false;

UPDATE expense_categories SET is_people_pay = true
WHERE code IN (
    'assistant',      -- помощник, единственная ежемесячная выплата одному человеку
    'ppc_specialist', -- Алсана, контекст
    'copywriting',
    'smm',
    'layout',
    'developer',
    'design',
    'advocates'
);
