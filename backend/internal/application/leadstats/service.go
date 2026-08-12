package leadstats

import (
	"context"
	"fmt"
	"time"

	domain "multiagent-seo/internal/domain/leadstats"
)

type Service struct {
	repo domain.Repository
}

func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// GetStats fetches everything the dashboard needs for one date range in one
// call — from/to are calendar dates (no time-of-day); to is widened to the
// end of that day so "to=2025-01-31" actually includes the 31st.
func (s *Service) GetStats(ctx context.Context, from, to time.Time, groupBy string) (domain.Stats, error) {
	if groupBy != "month" {
		groupBy = "day" // whitelist: anything else (including empty) defaults to day
	}
	toInclusive := to.Add(24*time.Hour - time.Nanosecond)

	totals, err := s.repo.Totals(ctx, from, toInclusive)
	if err != nil {
		return domain.Stats{}, fmt.Errorf("leadstats: totals: %w", err)
	}
	trend, err := s.repo.Trend(ctx, from, toInclusive, groupBy)
	if err != nil {
		return domain.Stats{}, fmt.Errorf("leadstats: trend: %w", err)
	}
	byPage, err := s.repo.ByPage(ctx, from, toInclusive)
	if err != nil {
		return domain.Stats{}, fmt.Errorf("leadstats: by page: %w", err)
	}
	byCreator, err := s.repo.ByCreator(ctx, from, toInclusive)
	if err != nil {
		return domain.Stats{}, fmt.Errorf("leadstats: by creator: %w", err)
	}
	byStatus, err := s.repo.ByConsultationStatus(ctx, from, toInclusive)
	if err != nil {
		return domain.Stats{}, fmt.Errorf("leadstats: by status: %w", err)
	}

	return domain.Stats{
		From:      from,
		To:        to,
		GroupBy:   groupBy,
		Totals:    totals,
		Trend:     trend,
		ByPage:    byPage,
		ByCreator: byCreator,
		ByStatus:  byStatus,
	}, nil
}
