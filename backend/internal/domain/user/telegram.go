package user

import "context"

// TelegramSubject is who a verified Mini App launch speaks for.
//
// Not a User: clients and advocates are recognised by the chat the bot already
// talks to, so there is nothing to look up in users and no password involved.
type TelegramSubject struct {
	Role Role
	// Exactly one of these carries the row this caller speaks for, matching
	// Role. Both empty is a guest.
	ClientID   string
	AdvocateID string
}

// TelegramRepository resolves a Telegram user id to whoever it belongs to.
//
// The id Telegram signs into init data is a *user* id, while the CRM stores a
// *chat* id — the same number for a private chat, which is the only kind the
// bot has with a client, and the same assumption the bot itself already makes
// when it binds one (see the intake flow's "private-chat id == tapping user's
// id").
//
// Returns ErrNotFound when the id matches nobody; deciding that this means
// "guest" is the middleware's call, not the store's.
type TelegramRepository interface {
	FindByTelegramID(ctx context.Context, telegramID int64) (TelegramSubject, error)
}
