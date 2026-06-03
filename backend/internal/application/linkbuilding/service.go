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
	"sync"
	"time"

	domain "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/jobrunner"
)

const (
	// maxConcurrentSites bounds parallel fetch+classify work. Kept low because
	// the 70B-class models we target have ~12k TPM free-tier budgets, and 8
	// concurrent classifies easily punch through that into 429 storms; 2 keeps
	// us well under the per-minute window while still hiding most network
	// latency. Bump when running on dedicated/paid tiers.
	maxConcurrentSites = 2
	// resultFlushBatch writes results to the sheet as the run proceeds, so a job
	// timeout can't discard everything already computed.
	resultFlushBatch   = 25
	resultWriteTimeout = 30 * time.Second
)

// ErrNoSheet / ErrNoTopics guard the request; the HTTP layer maps them to 400.
var (
	ErrNoSheet  = errors.New("sheet name is required")
	ErrNoTopics = errors.New("at least one accepted topic is required")
)

// LLMDefaults names the provider/model a service should use when the request
// didn't pick its own. Mirrors apparticles.Defaults but kept tiny because
// linkbuilding services only need these two knobs.
type LLMDefaults struct {
	Provider string
	Model    string
}

// TopicClassifierBuilder constructs a fresh classifier bound to the given
// provider/model. Wiring supplies it once (closing over the LLMFactory);
// per-request overrides flow through the same builder so a single Qualify call
// can target a different model than the deployment-wide default.
type TopicClassifierBuilder func(provider, model string) (domain.TopicClassifier, error)

type Service struct {
	sites             domain.WebsiteSource
	fetcher           domain.PageFetcher
	classifierBuilder TopicClassifierBuilder
	defaults          LLMDefaults
	runner            jobrunner.JobRunner
	log               *slog.Logger
}

func NewService(
	sites domain.WebsiteSource,
	fetcher domain.PageFetcher,
	classifierBuilder TopicClassifierBuilder,
	defaults LLMDefaults,
	runner jobrunner.JobRunner,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		sites:             sites,
		fetcher:           fetcher,
		classifierBuilder: classifierBuilder,
		defaults:          defaults,
		runner:            runner,
		log:               log,
	}
}

// QualifyRequest is the use-case input. CandidateTopics is the set the classifier
// chooses from; when empty it falls back to AcceptedTopics. A site is suitable
// when its classified topic is in AcceptedTopics. Provider/Model override the
// LLMDefaults for this run only.
type QualifyRequest struct {
	Sheet           string
	AcceptedTopics  []string
	CandidateTopics []string
	Provider        string
	Model           string
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
		// Nothing to qualify — no job dispatched, nothing written back.
		return QualifyResult{Sheet: req.Sheet}, nil
	}

	provider := pickStr(req.Provider, s.defaults.Provider)
	model := pickStr(req.Model, s.defaults.Model)
	classifier, err := s.classifierBuilder(provider, model)
	if err != nil {
		return QualifyResult{}, fmt.Errorf("build classifier (%s/%s): %w", provider, model, err)
	}

	jobLog := s.log.With("sheet", req.Sheet, "websites", len(sites), "provider", provider, "model", model)
	s.runner.Go(ctx, func(bg context.Context) {
		s.qualifyAll(bg, jobLog, req.Sheet, sites, candidates, req.AcceptedTopics, classifier)
	})

	jobLog.InfoContext(ctx, "website qualification accepted")
	return QualifyResult{Sheet: req.Sheet, WebsitesQueued: len(sites)}, nil
}

func pickStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// qualifyAll processes sites with bounded parallelism and writes results to the
// sheet in batches as they complete. A per-site fetch/classify failure is
// non-fatal (that row is marked unsuitable); a context cancellation aborts the
// run without writing a misleading verdict for in-flight rows.
func (s *Service) qualifyAll(ctx context.Context, log *slog.Logger, sheet string, sites []domain.Website, candidates, accepted []string, classifier domain.TopicClassifier) {
	resultsCh := make(chan domain.Result)
	sem := make(chan struct{}, maxConcurrentSites)
	var wg sync.WaitGroup

	go func() {
		for _, w := range sites {
			select {
			case <-ctx.Done():
				wg.Wait()
				close(resultsCh)
				return
			case sem <- struct{}{}:
			}
			wg.Add(1)
			go func(w domain.Website) {
				defer wg.Done()
				defer func() { <-sem }()
				if res, ok := s.qualifyOne(ctx, log, w, candidates, accepted, classifier); ok {
					resultsCh <- res
				}
			}(w)
		}
		wg.Wait()
		close(resultsCh)
	}()

	batch := make([]domain.Result, 0, resultFlushBatch)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Detach from the job deadline so already-computed rows still persist even
		// if the job context has timed out.
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resultWriteTimeout)
		defer cancel()
		if err := s.sites.WriteResults(writeCtx, sheet, batch); err != nil {
			log.ErrorContext(ctx, "write results failed", "err", err, "batch", len(batch))
		}
		batch = batch[:0]
	}

	processed := 0
	for res := range resultsCh {
		batch = append(batch, res)
		processed++
		if len(batch) >= resultFlushBatch {
			flush()
		}
	}
	flush()

	if ctx.Err() != nil {
		log.WarnContext(ctx, "website qualification aborted", "processed", processed, "total", len(sites), "err", ctx.Err())
		return
	}
	log.InfoContext(ctx, "website qualification done", "processed", processed)
}

// qualifyOne runs the fetch→count→classify→decide pipeline for one site. ok is
// false when the context was cancelled mid-flight, so the caller skips writing a
// misleading verdict for that row.
func (s *Service) qualifyOne(ctx context.Context, log *slog.Logger, w domain.Website, candidates, accepted []string, classifier domain.TopicClassifier) (domain.Result, bool) {
	res := domain.Result{Row: w.Row, URL: w.URL}

	page, err := s.fetcher.Fetch(ctx, w.URL)
	if err != nil {
		if isCanceled(ctx, err) {
			return domain.Result{}, false
		}
		log.WarnContext(ctx, "fetch failed, marking unsuitable", "url", w.URL, "err", err)
		return res, true
	}

	res.OutboundDomains = domain.CountExternalDomains(w.URL, page.Links)

	topic, err := classifier.Classify(ctx, page, candidates)
	if err != nil {
		if isCanceled(ctx, err) {
			return domain.Result{}, false
		}
		// A non-fatal classify failure leaves the topic empty (→ not suitable) but
		// keeps the outbound count we already have.
		log.WarnContext(ctx, "classify failed", "url", w.URL, "err", err)
	} else {
		res.Topic = topic
	}
	res.Suitable = domain.IsSuitable(res.Topic, accepted)

	log.DebugContext(ctx, "site qualified", "url", w.URL, "topic", res.Topic, "outbound", res.OutboundDomains, "suitable", res.Suitable)
	return res, true
}

// isCanceled reports whether the job context is done or err is a
// cancellation/timeout, as opposed to a genuine per-site failure.
func isCanceled(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
