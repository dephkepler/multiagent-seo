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

	// Empty right after Parse — Store.ResolveClient fills it in (needs a DB lookup).
	ClientID string
}

// ShortID drops the "@domain" suffix — nicer for a human to reference than
// a full email Message-ID.
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
