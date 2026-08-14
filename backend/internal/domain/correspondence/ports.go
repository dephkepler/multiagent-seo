package correspondence

import "context"

type Store interface {
	// UpsertContact creates or refreshes a contact by their Telegram user id
	// and returns the stored row (id included) — callers need the id to
	// attach messages.
	UpsertContact(ctx context.Context, c Contact) (Contact, error)
	// SaveMessage is a no-op if this (contact, telegram message id) pair was
	// already saved, so re-running an import never duplicates history.
	SaveMessage(ctx context.Context, m Message) error
}
