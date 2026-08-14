// Package correspondence holds the people davyd has talked to directly on
// his personal Telegram account, and their message history — separate from
// the Abalis bot's own leads/clients, which come through the bot instead.
package correspondence

import "time"

type Direction string

const (
	DirectionIn  Direction = "in"
	DirectionOut Direction = "out"
)

type Contact struct {
	ID             string
	TelegramUserID int64
	Username       string
	FirstName      string
	LastName       string
	Phone          string
	ClientID       string
	FirstSeenAt    time.Time
	LastSeenAt     time.Time
}

type Message struct {
	ID                string
	ContactID         string
	TelegramMessageID int64
	Direction         Direction
	Text              string
	SentAt            time.Time
}
