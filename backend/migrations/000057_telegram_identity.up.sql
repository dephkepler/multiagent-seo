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
-- Any chat id already sitting on more than one client row has to go before the
-- index can exist, or this migration fails on a deployed database — and a
-- failed migration leaves golang-migrate's version dirty, which is a manual
-- repair on a server rather than an error anyone sees.
--
-- The row that keeps the chat is the one seen most recently, which is the same
-- rule SetClientTelegram applies: the chat follows the client row actually
-- being used. A row that loses it is not damaged, it just stops being
-- reachable from the Mini App until that client opens it again.
UPDATE clients
SET telegram_chat_id = NULL
WHERE telegram_chat_id IS NOT NULL
	AND id NOT IN (
		SELECT DISTINCT ON (telegram_chat_id) id
		FROM clients
		WHERE telegram_chat_id IS NOT NULL
		ORDER BY telegram_chat_id, last_seen_at DESC, id
	);

UPDATE advocates a
SET telegram_chat_id = NULL
WHERE a.telegram_chat_id IS NOT NULL
	AND a.id NOT IN (
		SELECT DISTINCT ON (telegram_chat_id) id
		FROM advocates
		WHERE telegram_chat_id IS NOT NULL
		ORDER BY telegram_chat_id, created_at DESC, id
	);

CREATE UNIQUE INDEX IF NOT EXISTS uq_clients_telegram_chat_id
	ON clients (telegram_chat_id)
	WHERE telegram_chat_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_advocates_telegram_chat_id
	ON advocates (telegram_chat_id)
	WHERE telegram_chat_id IS NOT NULL;
