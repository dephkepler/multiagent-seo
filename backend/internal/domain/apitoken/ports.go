package apitoken

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("api token not found")

type NewToken struct {
	UserID uuid.UUID
	Name   string
	Hash   string
	Prefix string
}

type Repository interface {
	Create(ctx context.Context, t NewToken) (Token, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Token, error)
	UserByHash(ctx context.Context, hash string) (uuid.UUID, error)
	Revoke(ctx context.Context, userID, id uuid.UUID) error
}
