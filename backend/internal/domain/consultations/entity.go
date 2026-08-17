package consultations

import (
	"strings"
	"time"
)

// Status values a consultation can hold. Every consultation starts
// Scheduled (the DB column defaults to it) — staff move it to one of the
// other three from the inline buttons the bot sends after booking.
const (
	StatusScheduled = "scheduled"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
	StatusNoShow    = "no_show"
)

type Consultation struct {
	ID          string
	ClientID    string
	ScheduledAt time.Time
	Price       float64
	CaseNote    string
	CreatedBy   string
	Status      string
}

type Client struct {
	ID             string
	Name           string
	Phone          string
	TelegramName   string
	TelegramChatID int64
}

// ClientEdit is what staff can change about a client's own contact details
// from the client card — see clientdetail.Service.UpdateClient.
type ClientEdit struct {
	LastName   string
	FirstName  string
	Patronymic string
	Phone      string
}

// ComposeName joins name parts into one display string in Прізвище Ім'я
// По батькові order (Ukrainian legal/business convention) — every other
// place in the app (bot messages, search, cases) keeps reading the single
// Client.Name it produces, so this is the one place "full name" logic
// lives, not duplicated at each call site. Empty parts are skipped, not
// left as gaps.
func ComposeName(lastName, firstName, patronymic string) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{lastName, firstName, patronymic} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

type ReminderTarget struct {
	Consultation Consultation
	Client       Client
}

type Advocate struct {
	ID               string
	FullName         string
	TelegramUsername string
	TelegramChatID   int64
	// IsActive is false once an advocate has left — they drop out of
	// pickers for new work but stay on old cases/consultations untouched.
	IsActive bool
}
