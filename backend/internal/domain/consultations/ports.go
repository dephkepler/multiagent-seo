package consultations

import (
	"context"
	"time"
)

type Store interface {
	FindClient(ctx context.Context, clientID string) (Client, error)
	// SearchClients matches name, telegram name, or phone against query —
	// used by /creategroup so staff can find a client by typing what they
	// remember instead of copying a Client ID.
	SearchClients(ctx context.Context, query string) ([]Client, error)
	SetClientTelegram(ctx context.Context, clientID string, chatID int64, telegramName string) error
	Save(ctx context.Context, c Consultation) (Consultation, error)
	LatestConsultation(ctx context.Context, clientID string) (Consultation, error)
	UpdateStatus(ctx context.Context, consultationID, status string) error
	UpsertAdvocate(ctx context.Context, fullName string) (Advocate, error)
	SetAdvocateTelegram(ctx context.Context, chatID int64, telegramName string) error
	GetAdvocate(ctx context.Context) (Advocate, error)
	DueClientReminders(ctx context.Context, before time.Duration) ([]ReminderTarget, error)
	DueAdvocateReminders(ctx context.Context, before time.Duration) ([]ReminderTarget, error)
	MarkClientReminderSent(ctx context.Context, consultationID string) error
	MarkReminderSent(ctx context.Context, consultationID string) error
}

type SheetWriter interface {
	AppendRow(ctx context.Context, c Consultation, client Client) error
}
