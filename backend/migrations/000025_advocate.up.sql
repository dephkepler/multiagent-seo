ALTER TABLE consultations ADD COLUMN IF NOT EXISTS case_note text NOT NULL DEFAULT '';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS telegram_name text;

CREATE TABLE IF NOT EXISTS advocates (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name         text NOT NULL,
    telegram_username text,
    telegram_chat_id  bigint,
    created_at        timestamptz NOT NULL DEFAULT now()
);
