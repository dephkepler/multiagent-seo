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
	// UpdateClient edits a client's own contact details — name/phone typed
	// in through the admin UI's client card, not anything the bot or a
	// lead form fills in.
	UpdateClient(ctx context.Context, clientID, name, phone string) error
	Save(ctx context.Context, c Consultation) (Consultation, error)
	LatestConsultation(ctx context.Context, clientID string) (Consultation, error)
	UpdateStatus(ctx context.Context, consultationID, status string) error
	// CreateAdvocate always adds a new advocate — advocates are a roster,
	// not a single slot (see ABL 017's original one-advocate note, since
	// superseded).
	CreateAdvocate(ctx context.Context, fullName string) (Advocate, error)
	// ListAdvocates returns active-only advocates when activeOnly is true —
	// that's what pickers (/case, /creategroup) should show, since an
	// inactive advocate shouldn't get new work assigned.
	ListAdvocates(ctx context.Context, activeOnly bool) ([]Advocate, error)
	// DeactivateAdvocate is the "left the firm" action — the row (and every
	// case/consultation already linked to it) stays untouched, the advocate
	// just stops showing up in pickers for new work.
	DeactivateAdvocate(ctx context.Context, advocateID string) error
	SetAdvocateTelegram(ctx context.Context, advocateID string, chatID int64, telegramName string) error
	// GetAdvocate is a stopgap for the reminder loop, which isn't
	// per-advocate yet — returns the first active advocate, arbitrarily.
	GetAdvocate(ctx context.Context) (Advocate, error)
	DueClientReminders(ctx context.Context, before time.Duration) ([]ReminderTarget, error)
	DueAdvocateReminders(ctx context.Context, before time.Duration) ([]ReminderTarget, error)
	MarkClientReminderSent(ctx context.Context, consultationID string) error
	MarkReminderSent(ctx context.Context, consultationID string) error
}

type SheetWriter interface {
	AppendRow(ctx context.Context, c Consultation, client Client) error
}
