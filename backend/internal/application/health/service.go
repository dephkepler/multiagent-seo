package health

import (
	"context"

	"multiagent-seo/internal/domain/health"
)

type Service struct {
	domain *health.DomainService
}

func NewService(domain *health.DomainService) *Service {
	return &Service{domain: domain}
}

func (s *Service) Check(ctx context.Context) health.Health {
	return s.domain.Check(ctx)
}
