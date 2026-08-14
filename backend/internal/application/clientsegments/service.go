package clientsegments

import (
	"context"
	"fmt"

	domain "multiagent-seo/internal/domain/clientsegments"
)

type Service struct {
	repo domain.Repository
}

func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// List returns every client's current segment — no date range, unlike
// leadstats: a client's funnel position is a snapshot of everything that's
// ever happened to them, not something that makes sense sliced by period.
func (s *Service) List(ctx context.Context) ([]domain.ClientSegment, error) {
	activity, err := s.repo.ListActivity(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientsegments: list activity: %w", err)
	}
	out := make([]domain.ClientSegment, len(activity))
	for i, a := range activity {
		out[i] = domain.Derive(a)
	}
	return out, nil
}
