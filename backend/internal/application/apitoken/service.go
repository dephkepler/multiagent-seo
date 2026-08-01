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
