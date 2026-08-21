package user

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("user not found")

type Repository interface {
	FindByEmail(ctx context.Context, email string) (User, error)
	// FindByID resolves the caller of a request: the token carries an id and
	// nothing else, so the role has to be read from the row on every request.
	// Cheaper than it sounds at this scale, and it means a role change takes
	// effect immediately instead of when the token expires.
	FindByID(ctx context.Context, id string) (User, error)
	List(ctx context.Context) ([]User, error)
}
