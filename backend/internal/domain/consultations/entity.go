package consultations

import "time"

type Consultation struct {
	ID          string
	ClientID    string
	ScheduledAt time.Time
	Price       float64
	CaseNote    string
	CreatedBy   string
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
