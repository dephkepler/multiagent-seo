package webleads

import "context"

type MailSource interface {
	FetchUnseen(ctx context.Context) ([]Message, error)
	MarkSeen(ctx context.Context, uid uint32) error
}

type Notifier interface {
	// SendMessage sends text with an optional inline keyboard (nil/empty
	// for none) and returns the sent message's Telegram id — see
	// Lead.TelegramMessageID for why the caller needs it.
	SendMessage(ctx context.Context, text string, buttons []InlineButton) (telegramMessageID int, err error)
}

type Store interface {
	ResolveClient(ctx context.Context, phone, name string) (clientID string, err error)
	Save(ctx context.Context, lead Lead) error
	MarkSheetSynced(ctx context.Context, messageID string) error
	// SetPracticeArea records what a lead is about, keyed by the Telegram
	// message id it was announced in (Lead.TelegramMessageID) — a lead has
	// no id exposed to this layer, and MessageID is an email header of
	// unpredictable length, unsafe to carry in a Telegram callback_data
	// (64-byte cap). The tap that calls this always originates from that
	// exact message, so it's the only identifier that's both available and
	// safe to key off.
	SetPracticeArea(ctx context.Context, telegramMessageID int, area string) error
}

type SheetWriter interface {
	AppendRow(ctx context.Context, lead Lead) error
}
