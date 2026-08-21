package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"multiagent-seo/internal/domain/auth"
	"multiagent-seo/internal/domain/user"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

// Session is what a successful login hands the browser. Role travels with the
// token because the UI has to know which section to open — it is not what
// grants access: every request's role is read from the user row again in the
// auth middleware, so a client that edits its stored role gains nothing.
type Session struct {
	Token     string
	ExpiresAt time.Time
	Role      user.Role
}

type Service struct {
	users  user.Repository
	issuer auth.TokenIssuer
}

func NewService(users user.Repository, issuer auth.TokenIssuer) *Service {
	return &Service{users: users, issuer: issuer}
}

func (s *Service) ListUsers(ctx context.Context) ([]user.User, error) {
	return s.users.List(ctx)
}

func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, fmt.Errorf("login: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return Session{}, ErrInvalidCredentials
	}

	token, expiresAt, err := s.issuer.Issue(ctx, u.ID.String())
	if err != nil {
		return Session{}, fmt.Errorf("login: %w", err)
	}
	return Session{Token: token, ExpiresAt: expiresAt, Role: u.Role}, nil
}
