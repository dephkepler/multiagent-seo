// Package vault is a plain password/credential store for staff — one flat
// card per login, modeled after a Google Passwords-style list. Deliberately
// no encryption at rest yet: everything is stored and returned as plain
// text, same as the credentials it holds today would otherwise sit in a
// shared doc. Gated by nothing beyond the app's normal "must be logged in"
// check — there's no separate admin role in this app (see
// infrastructure/http/middleware.BearerAuth).
package vault

import (
	"time"

	"github.com/google/uuid"
)

// Entry is one saved login: a site/service name, where to log in, and the
// credentials for it.
type Entry struct {
	ID        uuid.UUID
	GroupID   uuid.UUID
	Title     string
	URL       string
	Username  string
	Password  string
	Notes     string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Group is a named bucket of entries — e.g. "Соцсети" holding all
// social-media logins, instead of everything in one flat list.
type Group struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

// GroupWithCount is what the groups list screen needs — a group plus how
// many entries it holds, computed by ListGroups (not stored).
type GroupWithCount struct {
	Group
	EntryCount int
}
