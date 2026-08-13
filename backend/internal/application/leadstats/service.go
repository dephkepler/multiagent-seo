package leadstats

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	domain "multiagent-seo/internal/domain/leadstats"
)

type Service struct {
	repo    domain.Repository
	traffic domain.TrafficSource // nil if GA4 isn't configured — every traffic field just stays 0
	log     *slog.Logger
}

func NewService(repo domain.Repository, traffic domain.TrafficSource, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, traffic: traffic, log: log}
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
	byCategory, err := s.repo.ByCaseCategory(ctx, from, toInclusive)
	if err != nil {
		return domain.Stats{}, fmt.Errorf("leadstats: by category: %w", err)
	}

	s.mergeTraffic(ctx, from, toInclusive, groupBy, &totals, trend)

	return domain.Stats{
		From:       from,
		To:         to,
		GroupBy:    groupBy,
		Totals:     totals,
		Trend:      trend,
		ByPage:     byPage,
		ByCreator:  byCreator,
		ByStatus:   byStatus,
		ByCategory: byCategory,
	}, nil
}

// mergeTraffic folds GA4 sessions into the already-built trend buckets (by
// matching Bucket key) and sums them into Totals — a GA4 failure only logs
// and leaves traffic fields at 0, it never fails the whole dashboard, same
// as how a sheet-write failure never blocks a lead in ProcessNewLeads.
func (s *Service) mergeTraffic(ctx context.Context, from, to time.Time, groupBy string, totals *domain.Totals, trend []domain.Bucket) {
	if s.traffic == nil {
		return
	}
	buckets, err := s.traffic.SessionsByPeriod(ctx, from, to, groupBy)
	if err != nil {
		s.log.WarnContext(ctx, "leadstats: ga4 sessions failed, showing dashboard without traffic", "err", err)
		return
	}

	byKey := make(map[string]domain.TrafficBucket, len(buckets))
	for _, b := range buckets {
		byKey[b.Bucket] = b
	}
	for i := range trend {
		if b, ok := byKey[trend[i].Bucket]; ok {
			trend[i].SiteSessions = b.Sessions
			trend[i].OrganicSessions = b.OrganicSessions
			totals.SiteSessions += b.Sessions
			totals.OrganicSessions += b.OrganicSessions
		}
	}
}
