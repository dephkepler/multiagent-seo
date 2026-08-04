package webleads

import "context"

type MailSource interface {
	FetchUnseen(ctx context.Context) ([]Message, error)
	MarkSeen(ctx context.Context, uid uint32) error
}

type Notifier interface {
	SendMessage(ctx context.Context, text string) error
}

type Store interface {
	ResolveClient(ctx context.Context, phone, name string) (clientID string, err error)
	Save(ctx context.Context, lead Lead) error
	MarkSheetSynced(ctx context.Context, messageID string) error
}

type SheetWriter interface {
	AppendRow(ctx context.Context, lead Lead) error
}
