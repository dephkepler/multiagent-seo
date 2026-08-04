ALTER TABLE clients ADD COLUMN IF NOT EXISTS telegram_chat_id bigint;

CREATE TABLE IF NOT EXISTS consultations (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id                uuid NOT NULL REFERENCES clients(id),
    scheduled_at             timestamptz NOT NULL,
    price                    numeric NOT NULL,
    created_by               text NOT NULL DEFAULT '',
    reminder_sent_at         timestamptz,
    client_reminder_sent_at  timestamptz,
    created_at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_consultations_client_id ON consultations (client_id);
