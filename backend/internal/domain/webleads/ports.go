package webleads

import "context"

type MailSource interface {
	FetchUnseen(ctx context.Context) ([]Message, error)
	MarkSeen(ctx context.Context, uid uint32) error
}

type Notifier interface {
	SendMessage(ctx context.Context, text string) error
}

// Save is idempotent on MessageID: saving the same lead twice is a no-op.
type Store interface {
	// ResolveClient must run before the Telegram send (so ClientID can be
	// shown there); a failure here is non-fatal and never blocks delivery.
	ResolveClient(ctx context.Context, phone, name string) (clientID string, err error)
	Save(ctx context.Context, lead Lead) error
	MarkSheetSynced(ctx context.Context, messageID string) error
}

type SheetWriter interface {
	AppendRow(ctx context.Context, lead Lead) error
}
