// Package linkbuilding is the application-layer use-case that qualifies donor
// websites: read a list from a sheet, classify each one's topic, count its
// outbound domains, decide suitability, and write the results back. It owns no
// infrastructure — every dependency is a domain port injected via the constructor.
package linkbuilding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	domain "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/jobrunner"
)

// ErrNoSheet / ErrNoTopics guard the request; the HTTP layer maps them to 400.
var (
	ErrNoSheet  = errors.New("sheet name is required")
	ErrNoTopics = errors.New("at least one accepted topic is required")
)

type Service struct {
	sites      domain.WebsiteSource
	fetcher    domain.PageFetcher
	classifier domain.TopicClassifier
	runner     jobrunner.JobRunner
	log        *slog.Logger
}

func NewService(
	sites domain.WebsiteSource,
	fetcher domain.PageFetcher,
	classifier domain.TopicClassifier,
	runner jobrunner.JobRunner,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{sites: sites, fetcher: fetcher, classifier: classifier, runner: runner, log: log}
}

// QualifyRequest is the use-case input. CandidateTopics is the set the classifier
// chooses from; when empty it falls back to AcceptedTopics. A site is suitable
// when its classified topic is in AcceptedTopics.
type QualifyRequest struct {
	Sheet           string
	AcceptedTopics  []string
	CandidateTopics []string
}

// QualifyResult is the synchronous 202-style outcome: how many websites were
// read and queued. Per-site results land in the sheet as the job runs.
type QualifyResult struct {
	Sheet          string
	WebsitesQueued int
}

// QualifyWebsites reads the website list, then dispatches the per-site
// fetch→count→classify→decide→write pipeline through the JobRunner and returns
// immediately. A SyncRunner runs it inline (test seam).
func (s *Service) QualifyWebsites(ctx context.Context, req QualifyRequest) (QualifyResult, error) {
	if req.Sheet == "" {
		return QualifyResult{}, ErrNoSheet
	}
	if len(req.AcceptedTopics) == 0 {
		return QualifyResult{}, ErrNoTopics
	}
	candidates := req.CandidateTopics
	if len(candidates) == 0 {
		candidates = req.AcceptedTopics
	}

	sites, err := s.sites.List(ctx, req.Sheet)
	if err != nil {
		return QualifyResult{}, fmt.Errorf("list websites: %w", err)
	}
	if len(sites) == 0 {
		return QualifyResult{Sheet: req.Sheet}, nil
	}

	jobLog := s.log.With("sheet", req.Sheet, "websites", len(sites))
	s.runner.Go(ctx, func(bg context.Context) {
		s.qualifyAll(bg, jobLog, req.Sheet, sites, candidates, req.AcceptedTopics)
	})

	jobLog.InfoContext(ctx, "website qualification accepted")
	return QualifyResult{Sheet: req.Sheet, WebsitesQueued: len(sites)}, nil
}

// qualifyAll processes every site and writes all results back in one batch. A
// per-site fetch/classify failure is non-fatal: that row is marked unsuitable
// (empty topic) and the run continues.
func (s *Service) qualifyAll(ctx context.Context, log *slog.Logger, sheet string, sites []domain.Website, candidates, accepted []string) {
	results := make([]domain.Result, 0, len(sites))
	for _, w := range sites {
		res := domain.Result{Row: w.Row, URL: w.URL}

		page, err := s.fetcher.Fetch(ctx, w.URL)
		if err != nil {
			log.WarnContext(ctx, "fetch failed, marking unsuitable", "url", w.URL, "err", err)
			results = append(results, res)
			continue
		}

		res.OutboundDomains = domain.CountExternalDomains(w.URL, page.Links)

		topic, err := s.classifier.Classify(ctx, page, candidates)
		if err != nil {
			// Classification failure leaves the topic empty (→ not suitable) but
			// keeps the outbound count we already have.
			log.WarnContext(ctx, "classify failed", "url", w.URL, "err", err)
		} else {
			res.Topic = topic
		}
		res.Suitable = domain.IsSuitable(res.Topic, accepted)

		log.DebugContext(ctx, "site qualified", "url", w.URL, "topic", res.Topic, "outbound", res.OutboundDomains, "suitable", res.Suitable)
		results = append(results, res)
	}

	if err := s.sites.WriteResults(ctx, sheet, results); err != nil {
		log.ErrorContext(ctx, "write results failed", "err", err)
		return
	}
	log.InfoContext(ctx, "website qualification done", "processed", len(results))
}
