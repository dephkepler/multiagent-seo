CREATE TABLE IF NOT EXISTS staff_prefs (
    telegram_user_id bigint PRIMARY KEY,
    language          text        NOT NULL DEFAULT 'uk' CHECK (language IN ('uk', 'ru')),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
