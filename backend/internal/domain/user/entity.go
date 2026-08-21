package user

import (
	"time"

	"github.com/google/uuid"
)

// Role decides what a session may reach. There was no role at all until an
// advocate needed a login: every authenticated caller saw the P&L, every
// contractor's pay and the password vault.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleAdvocate Role = "advocate"
	// RoleClient and RoleGuest are callers Telegram vouched for, never rows in
	// users: a client has no password and never will.
	//
	// A guest is a verified launch that matched nobody — someone opening the
	// app before the CRM knows them. It is a role of its own rather than a
	// client with an empty id so that the scope gate refuses it everywhere by
	// default, and only the intake operation — the one that creates the client
	// — has to name it. The alternative puts an "is the id empty" check in
	// every client handler, and the one that forgets it queries with an empty
	// client id.
	RoleClient Role = "client"
	RoleGuest  Role = "guest"
)

// IsRole answers what may be stored on a users row, which is why the Telegram
// roles are absent: createuser must keep refusing to mint a client, since a
// client is identified by the chat the bot talks to and has no password to
// store.
func IsRole(r Role) bool {
	return r == RoleAdmin || r == RoleAdvocate
}

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	Role         Role
	// AdvocateID is the roster row this login speaks for — the key every
	// advocate-scoped query filters on. Empty for admins, who are not on the
	// roster.
	AdvocateID string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
