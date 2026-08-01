CREATE TABLE IF NOT EXISTS leads (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id        text        NOT NULL,
    received_at       timestamptz NOT NULL,
    from_email        text        NOT NULL DEFAULT '',
    subject           text        NOT NULL DEFAULT '',
    name              text        NOT NULL DEFAULT '',
    phone             text        NOT NULL DEFAULT '',
    message           text        NOT NULL DEFAULT '',
    page              text        NOT NULL DEFAULT '',
    raw_body          text        NOT NULL DEFAULT '',
    telegram_sent_at  timestamptz,
    sheet_synced_at   timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_leads_message_id ON leads (message_id);
