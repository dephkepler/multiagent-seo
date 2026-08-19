-- Staff creating an old client through the bot may not have a phone for
-- them at all (only an email or Telegram handle, or nothing yet) — phone
-- used to be the one required, unique identity a client row was upserted
-- against (see ResolveClient/UpsertClient), which made "no phone" simply
-- impossible to record. Drop NOT NULL, and replace the plain unique index
-- with a partial one that only enforces uniqueness among clients that
-- actually have a phone — any number of "no phone yet" clients can now
-- coexist without colliding with each other on ON CONFLICT (phone).
ALTER TABLE clients ALTER COLUMN phone DROP NOT NULL;

DROP INDEX IF EXISTS uq_clients_phone;
CREATE UNIQUE INDEX IF NOT EXISTS uq_clients_phone ON clients (phone) WHERE phone IS NOT NULL AND phone <> '';
