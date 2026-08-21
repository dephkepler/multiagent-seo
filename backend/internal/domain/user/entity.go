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
)

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
