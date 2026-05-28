package auth

import (
	"context"
	"time"
)

type TokenIssuer interface {
	Issue(ctx context.Context, userID string) (token string, expiresAt time.Time, err error)
}

type TokenVerifier interface {
	Verify(ctx context.Context, token string) (userID string, err error)
}
