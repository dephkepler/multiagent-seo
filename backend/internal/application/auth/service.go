package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"contentflow/internal/domain/auth"
	"contentflow/internal/domain/user"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type Service struct {
	users  user.Repository
	issuer auth.TokenIssuer
}

func NewService(users user.Repository, issuer auth.TokenIssuer) *Service {
	return &Service{users: users, issuer: issuer}
}

func (s *Service) Login(ctx context.Context, email, password string) (string, time.Time, error) {
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		// A missing user and a wrong password are reported identically so the
		// response can't be used to enumerate which emails are registered.
		if errors.Is(err, user.ErrNotFound) {
			return "", time.Time{}, ErrInvalidCredentials
		}
		return "", time.Time{}, fmt.Errorf("login: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", time.Time{}, ErrInvalidCredentials
	}

	token, expiresAt, err := s.issuer.Issue(ctx, u.ID.String())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("login: %w", err)
	}
	return token, expiresAt, nil
}
