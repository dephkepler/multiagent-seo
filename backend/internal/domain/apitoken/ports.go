package apitoken

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("api token not found")

// NewToken is the data persisted for a freshly minted key. Hash is the sha256 of
// the secret; the secret itself is returned to the caller once and never stored.
type NewToken struct {
	UserID uuid.UUID
	Name   string
	Hash   string
	Prefix string
}

type Repository interface {
	Create(ctx context.Context, t NewToken) (Token, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Token, error)
	// UserByHash returns the owner of an active (non-revoked) token, or
	// ErrNotFound — used to authenticate a presented API key.
	UserByHash(ctx context.Context, hash string) (uuid.UUID, error)
	Revoke(ctx context.Context, userID, id uuid.UUID) error
}
