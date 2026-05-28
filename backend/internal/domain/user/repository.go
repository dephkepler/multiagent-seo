package user

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("user not found")

type Repository interface {
	FindByEmail(ctx context.Context, email string) (User, error)
}
