package webleads

import (
	"strings"
	"time"
)

type Lead struct {
	MessageID  string
	ReceivedAt time.Time
	FromEmail  string
	Subject    string
	Name       string
	Phone      string
	Message    string
	Page       string
	RawBody    string

	// Bare username (no "@"), set only for leads submitted through the
	// Telegram bot itself — empty for email-sourced leads.
	TelegramUsername string

	// Empty right after Parse — Store.ResolveClient fills it in (needs a DB lookup).
	ClientID string
}

func (l Lead) ShortID() string {
	id, _, _ := strings.Cut(l.MessageID, "@")
	return id
}

type Message struct {
	UID       uint32
	MessageID string
	From      string
	Subject   string
	Date      time.Time
	Body      string
}
