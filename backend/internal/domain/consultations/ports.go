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
	// UpsertClient creates a client if none exists for this phone, or just
	// touches last_seen_at (and fills in name only if it was blank) if one
	// already does — same idempotent-by-phone pattern webleads.ResolveClient
	// uses, exposed here because /book and /case need the resulting Client
	// row immediately (not just its id) to hand a Client ID staff never had
	// to know in the first place.
	UpsertClient(ctx context.Context, name, phone string) (Client, error)
	// SetClientEmail sets a single field — unlike UpdateClient, which
	// overwrites the whole card. The self-service intake flow only ever
	// learns an email (name/phone go through webleads.ResolveClient
	// instead); calling UpdateClient with everything else blank would wipe
	// the name/phone that call just set.
	SetClientEmail(ctx context.Context, clientID, email string) error
	// UpdateClient edits a client's own contact details — typed in through
	// the admin UI's client card, not anything the bot or a lead form
	// fills in.
	UpdateClient(ctx context.Context, clientID string, edit ClientEdit) error
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
