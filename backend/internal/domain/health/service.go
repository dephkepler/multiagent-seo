package health

import "context"

type DomainService struct {
	repo Repository
}

func NewService(repo Repository) *DomainService {
	return &DomainService{repo: repo}
}

func (s *DomainService) Check(ctx context.Context) Health {
	if s.repo == nil {
		return NewHealth(DBStatusUnknown)
	}
	if err := s.repo.Ping(ctx); err != nil {
		return NewHealth(DBStatusDown)
	}
	return NewHealth(DBStatusOK)
}
