-- Structured name parts, editable separately on the client card — `name`
-- stays as the single display string everywhere else in the app (bot
-- messages, search, cases) already reads it, computed from these three on
-- every save instead of duplicating "assemble a full name" logic at every
-- call site.
ALTER TABLE clients ADD COLUMN IF NOT EXISTS last_name text NOT NULL DEFAULT '';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS first_name text NOT NULL DEFAULT '';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS patronymic text NOT NULL DEFAULT '';
