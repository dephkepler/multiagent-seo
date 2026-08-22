package vault

import (
	"context"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/vault"
)

// Service is a thin pass-through to the repository — there's no business
// rule here beyond what the HTTP boundary already validates (title/password
// non-empty), same as wordpress.Service.
type Service struct {
	repo vault.Repository
}

func NewService(repo vault.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, in vault.CreateEntry) (vault.Entry, error) {
	return s.repo.Create(ctx, in)
}

func (s *Service) List(ctx context.Context, groupID uuid.UUID) ([]vault.Entry, error) {
	return s.repo.List(ctx, groupID)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in vault.UpdateEntry) (vault.Entry, error) {
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) CreateGroup(ctx context.Context, name string) (vault.Group, error) {
	return s.repo.CreateGroup(ctx, name)
}

func (s *Service) ListGroups(ctx context.Context) ([]vault.GroupWithCount, error) {
	return s.repo.ListGroups(ctx)
}

func (s *Service) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteGroup(ctx, id)
}
