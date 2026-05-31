package jwtauth

import (
	"context"
	"time"

	"multiagent-seo/pkg/jwt"
)

type Service struct {
	secret []byte
	ttl    time.Duration
}

func New(secret string, ttl time.Duration) *Service {
	return &Service{secret: []byte(secret), ttl: ttl}
}

func (s *Service) Issue(_ context.Context, userID string) (string, time.Time, error) {
	return jwt.Sign(userID, s.ttl, s.secret)
}

func (s *Service) Verify(_ context.Context, token string) (string, error) {
	return jwt.Parse(token, s.secret)
}
