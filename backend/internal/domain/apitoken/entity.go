// Package apitoken is the domain for long-lived, revocable API keys: per-user
// credentials stored as a hash (the secret is shown once at creation and never
// persisted in clear). It owns no infrastructure — persistence is a port.
package apitoken

import (
	"time"

	"github.com/google/uuid"
)

// Token is an API key's metadata. The secret is never held here after creation;
// only Prefix (a display fragment) and the stored hash identify it.
type Token struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	Prefix    string
	CreatedAt time.Time
	RevokedAt *time.Time
}
