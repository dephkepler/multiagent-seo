// Package apitoken is the use-case for minting, listing, revoking and
// authenticating per-user API keys. The secret is generated here, returned to
// the caller once, and only its sha256 hash is handed to the repository.
package apitoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	domain "multiagent-seo/internal/domain/apitoken"
)

// keyPrefix tags our keys so the auth middleware can tell an API key from a JWT.
const keyPrefix = "mas_"

var (
	ErrNoName     = errors.New("token name is required")
	ErrInvalidKey = errors.New("invalid api key")
)

type Service struct {
	repo domain.Repository
}

func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// Create mints a key for the user and returns the full secret ONCE plus the
// stored metadata. The secret cannot be retrieved afterwards (only its hash is
// persisted) — the caller must save it now.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, name string) (domain.Token, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Token{}, "", ErrNoName
	}
	secret, err := generateSecret()
	if err != nil {
		return domain.Token{}, "", fmt.Errorf("generate api key: %w", err)
	}
	t, err := s.repo.Create(ctx, domain.NewToken{
		UserID: userID,
		Name:   name,
		Hash:   hashKey(secret),
		Prefix: secret[:len(keyPrefix)+6],
	})
	if err != nil {
		return domain.Token{}, "", err
	}
	return t, secret, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]domain.Token, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *Service) Revoke(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.Revoke(ctx, userID, id)
}

// Authenticate resolves a presented API key to its owning user, or ErrInvalidKey.
func (s *Service) Authenticate(ctx context.Context, key string) (uuid.UUID, error) {
	if !HasKeyPrefix(key) {
		return uuid.Nil, ErrInvalidKey
	}
	uid, err := s.repo.UserByHash(ctx, hashKey(key))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return uuid.Nil, ErrInvalidKey
		}
		return uuid.Nil, err
	}
	return uid, nil
}

// HasKeyPrefix reports whether a bearer value is one of our API keys (vs a JWT),
// so the verifier can route it to Authenticate instead of JWT parsing.
func HasKeyPrefix(token string) bool {
	return strings.HasPrefix(token, keyPrefix)
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return keyPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
