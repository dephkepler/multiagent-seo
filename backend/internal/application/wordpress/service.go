package wordpress

import (
	"context"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/wordpress"
)

type Service struct {
	repo wordpress.Repository
}

func NewService(repo wordpress.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, in wordpress.CreateSite) (wordpress.Site, error) {
	return s.repo.Create(ctx, in)
}

func (s *Service) List(ctx context.Context) ([]wordpress.Site, error) {
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (wordpress.Site, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in wordpress.UpdateSite) (wordpress.Site, error) {
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
