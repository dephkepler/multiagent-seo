-- Company P&L: the expense half of what was tracked by hand in the
-- "Доходы/Расходы" spreadsheet since June 2024. The income half is NOT
-- duplicated here — it already lives in consultations.price and
-- case_payments.amount, and a second hand-typed copy of it would drift from
-- what /clients and the dashboard show.

-- Closed category vocabulary, same shape (and same reason) as
-- client_tag_defs: free-text categories drift into "реклама гугл" /
-- "Реклама Гугл" / "гугл адс" as three unrelated rows in a P&L. `code` is
-- the stable machine key, `label` is what staff sees and may rename freely.
CREATE TABLE IF NOT EXISTS expense_categories (
    code       text PRIMARY KEY,
    label      text        NOT NULL,
    kind       text        NOT NULL,
    is_active  boolean     NOT NULL DEFAULT true,
    sort_order int         NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- kind drives the derived numbers, not just grouping: 'marketing' is the
-- CAC/ROMI denominator, 'direct' is what's subtracted for gross margin
-- (advocate payouts scale with delivered work; rent and salaries don't).
ALTER TABLE expense_categories ADD CONSTRAINT expense_categories_kind_check
    CHECK (kind IN ('marketing', 'direct', 'payroll', 'development', 'infra', 'admin'));

INSERT INTO expense_categories (code, label, kind, sort_order) VALUES
    ('google_ads',     'Реклама Гугл',    'marketing',   10),
    ('ppc_specialist', 'Контекст (зп)',   'marketing',   20),
    ('copywriting',    'Копирайтинг',     'marketing',   30),
    ('smm',            'СММ',             'marketing',   40),
    ('advocates',      'Адвокаты',        'direct',      50),
    ('assistant',      'Помощник (зп)',   'payroll',     60),
    ('layout',         'Верстка',         'development', 70),
    ('developer',      'Программист',     'development', 80),
    ('design',         'Дизайн',          'development', 90),
    ('telephony',      'Телефония',       'infra',      100),
    ('website',        'Сайт',            'infra',      110),
    ('company',        'Компания',        'admin',      120),
    ('admin_misc',     'Админ. расходы',  'admin',      130)
ON CONFLICT (code) DO NOTHING;

-- Recurring templates — the "автоматические расходы" the cron materializes
-- into real expenses rows once a month (Алсана 9 140, хостинг 642,
-- Бинотель, Киевстар/Водафон, зп помощника: same vendor, same day, same
-- amount every month in the spreadsheet).
CREATE TABLE IF NOT EXISTS expense_rules (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name           text        NOT NULL,
    category_code  text        NOT NULL REFERENCES expense_categories(code) ON UPDATE CASCADE,
    vendor         text        NOT NULL DEFAULT '',
    payment_method text        NOT NULL DEFAULT 'card',
    -- expected amount; staff can correct it when confirming the generated draft.
    -- The checks below mirror `expenses` exactly: a rule that is legal here but
    -- illegal there would make the generator fail on every pass, blocking every
    -- later rule and every advocate payout with it.
    amount         numeric     NOT NULL CHECK (amount > 0),
    -- capped at 28 so a rule never silently skips February
    day_of_month   int         NOT NULL CHECK (day_of_month BETWEEN 1 AND 28),
    -- false: generate a draft to confirm. true: post straight to the ledger
    auto_post      boolean     NOT NULL DEFAULT false,
    active_from    date        NOT NULL DEFAULT current_date,
    active_to      date,
    is_active      boolean     NOT NULL DEFAULT true,
    created_by     text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE expense_rules ADD CONSTRAINT expense_rules_payment_method_check
    CHECK (payment_method IN ('card', 'invoice', 'company', 'cash'));

CREATE TABLE IF NOT EXISTS expenses (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    spent_at       date        NOT NULL,
    amount         numeric     NOT NULL CHECK (amount > 0),
    category_code  text        NOT NULL REFERENCES expense_categories(code) ON UPDATE CASCADE,
    payment_method text        NOT NULL DEFAULT 'card',
    vendor         text        NOT NULL DEFAULT '',
    description    text        NOT NULL DEFAULT '',
    status         text        NOT NULL DEFAULT 'posted',
    origin         text        NOT NULL DEFAULT 'manual',
    rule_id        uuid        REFERENCES expense_rules(id) ON DELETE SET NULL,
    -- Idempotency key for every non-manual row: 'rule:<uuid>:2026-08',
    -- 'advocate:<uuid>:2026-08', 'sheet:<tab>:<row>'. The unique index below
    -- is what lets the generator and the spreadsheet import re-run any
    -- number of times without charging the company twice.
    external_ref   text,
    created_by     text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- 'void' is a deletion that keeps its external_ref: dropping a generated row
-- outright frees the key, and the next generator pass re-creates the very
-- expense staff just removed. Void rows are excluded from every P&L number the
-- same way drafts are.
ALTER TABLE expenses ADD CONSTRAINT expenses_status_check
    CHECK (status IN ('draft', 'posted', 'void'));
ALTER TABLE expenses ADD CONSTRAINT expenses_origin_check
    CHECK (origin IN ('manual', 'recurring', 'derived', 'imported'));
ALTER TABLE expenses ADD CONSTRAINT expenses_payment_method_check
    CHECK (payment_method IN ('card', 'invoice', 'company', 'cash'));

CREATE UNIQUE INDEX IF NOT EXISTS uq_expenses_external_ref
    ON expenses (external_ref) WHERE external_ref IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_expenses_spent_at ON expenses (spent_at);
CREATE INDEX IF NOT EXISTS idx_expenses_status ON expenses (status) WHERE status = 'draft';

-- Money in that never passed through a consultation or a case: refunds,
-- "от компании" top-ups, one-offs. Without it the balance on the P&L page
-- can't match the real one, and staff would have no place to put it but a
-- fake consultation.
CREATE TABLE IF NOT EXISTS other_income (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    received_at date        NOT NULL,
    amount      numeric     NOT NULL CHECK (amount > 0),
    source      text        NOT NULL DEFAULT '',
    description text        NOT NULL DEFAULT '',
    created_by  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_other_income_received_at ON other_income (received_at);

-- Advocate payouts stop being one hand-typed monthly lump ("Оплата
-- адвокатам за сентябрь — 7 900") and become a share of what that advocate
-- actually collected, per case_payments. 0 means no auto-payout for this
-- advocate — nothing is guessed for the existing roster.
ALTER TABLE advocates ADD COLUMN IF NOT EXISTS commission_percent numeric NOT NULL DEFAULT 0;
ALTER TABLE advocates ADD CONSTRAINT advocates_commission_percent_check
    CHECK (commission_percent >= 0 AND commission_percent <= 100);
