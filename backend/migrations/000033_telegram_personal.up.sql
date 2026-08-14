-- People davyd has talked to directly on his personal Telegram account (not
-- through the Abalis bot) — keyed by Telegram's own user id so re-running the
-- importer upserts instead of duplicating contacts.
CREATE TABLE IF NOT EXISTS telegram_contacts (
	id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
	telegram_user_id bigint      NOT NULL,
	username         text        NOT NULL DEFAULT '',
	first_name       text        NOT NULL DEFAULT '',
	last_name        text        NOT NULL DEFAULT '',
	phone            text        NOT NULL DEFAULT '',
	client_id        uuid        REFERENCES clients(id),
	first_seen_at    timestamptz NOT NULL DEFAULT now(),
	last_seen_at     timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_telegram_contacts_user_id ON telegram_contacts (telegram_user_id);

-- One row per message, either direction. telegram_message_id is Telegram's
-- own per-chat message numbering, unique per contact, so re-importing the
-- same history never duplicates a row.
CREATE TABLE IF NOT EXISTS telegram_messages (
	id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
	contact_id          uuid        NOT NULL REFERENCES telegram_contacts(id),
	telegram_message_id bigint      NOT NULL,
	direction           text        NOT NULL,
	text                text        NOT NULL DEFAULT '',
	sent_at             timestamptz NOT NULL,
	created_at          timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE telegram_messages ADD CONSTRAINT telegram_messages_direction_check
	CHECK (direction IN ('in', 'out'));

CREATE UNIQUE INDEX IF NOT EXISTS uq_telegram_messages_contact_msg ON telegram_messages (contact_id, telegram_message_id);
CREATE INDEX IF NOT EXISTS idx_telegram_messages_contact_id ON telegram_messages (contact_id);
