-- telegram_chat_id stops being a note on a row and becomes an identity: the
-- Mini App has no password, so this column is what a request is authenticated
-- against. Two consequences the column was never set up for.
--
-- It is read on every request, so it needs an index — there was none at all
-- since 000024 added the column, and 658 of 659 client rows carry no chat id,
-- which is why the index is partial: only rows that can actually be looked up
-- belong in it.
--
-- And it must be unique, or "who is calling" has more than one answer. Postgres
-- allows repeated NULLs in a unique index, so unbound rows stay unaffected.
CREATE UNIQUE INDEX IF NOT EXISTS uq_clients_telegram_chat_id
	ON clients (telegram_chat_id)
	WHERE telegram_chat_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_advocates_telegram_chat_id
	ON advocates (telegram_chat_id)
	WHERE telegram_chat_id IS NOT NULL;
