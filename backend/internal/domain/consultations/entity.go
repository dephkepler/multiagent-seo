package consultations

import "time"

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

type ReminderTarget struct {
	Consultation Consultation
	Client       Client
}

type Advocate struct {
	ID               string
	FullName         string
	TelegramUsername string
	TelegramChatID   int64
}
