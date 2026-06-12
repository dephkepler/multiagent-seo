package linkbuilding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"

	domain "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/jobrunner"
)

const (
	maxConcurrentSites = 2
	resultFlushBatch   = 25
	resultWriteTimeout = 30 * time.Second
	maxPostsPerSite    = 3
	perPageChars       = 400
)

var (
	ErrNoSheet  = errors.New("sheet name is required")
	ErrNoTopics = errors.New("at least one accepted topic is required")
)

type LLMDefaults struct {
	Provider string
	Model    string
}

type TopicClassifierBuilder func(provider, model string) (domain.TopicClassifier, error)

type Service struct {
	sites             domain.WebsiteSource
	fetcher           domain.PageFetcher
	posts             domain.PostsDiscover
	classifierBuilder TopicClassifierBuilder
	defaults          LLMDefaults
	runner            jobrunner.JobRunner
	log               *slog.Logger
}

func NewService(
	sites domain.WebsiteSource,
	fetcher domain.PageFetcher,
	posts domain.PostsDiscover,
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
		posts:             posts,
		classifierBuilder: classifierBuilder,
		defaults:          defaults,
		runner:            runner,
		log:               log,
	}
}

type QualifyRequest struct {
	Sheet           string
	AcceptedTopics  []string
	CandidateTopics []string
	Provider        string
	Model           string
}

type QualifyResult struct {
	Sheet          string
	WebsitesQueued int
}

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

	provider := pickStr(req.Provider, s.defaults.Provider)
	model := req.Model
	if model == "" && provider == s.defaults.Provider {
		model = s.defaults.Model
	}
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
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resultWriteTimeout)
		defer cancel()
		if err := s.sites.WriteResults(writeCtx, sheet, batch); err != nil {
			return fmt.Errorf("write results (batch %d): %w", len(batch), err)
		}
		batch = batch[:0]
		return nil
	}

	processed := 0
	for res := range resultsCh {
		batch = append(batch, res)
		processed++
		if len(batch) >= resultFlushBatch {
			if err := flush(); err != nil {
				log.ErrorContext(ctx, "website qualification stopped on write failure", "processed", processed, "total", len(sites), "err", err)
				return
			}
		}
	}
	if err := flush(); err != nil {
		log.ErrorContext(ctx, "website qualification final write failed", "processed", processed, "total", len(sites), "err", err)
		return
	}

	if ctx.Err() != nil {
		log.WarnContext(ctx, "website qualification aborted", "processed", processed, "total", len(sites), "err", ctx.Err())
		return
	}
	log.InfoContext(ctx, "website qualification done", "processed", processed)
}

func (s *Service) qualifyOne(ctx context.Context, log *slog.Logger, w domain.Website, candidates, accepted []string, classifier domain.TopicClassifier) (domain.Result, bool) {
	res := domain.Result{Row: w.Row, URL: w.URL}

	home, err := s.fetcher.Fetch(ctx, w.URL)
	if err != nil {
		if isCanceled(ctx, err) {
			return domain.Result{}, false
		}
		log.WarnContext(ctx, "fetch failed, marking unsuitable", "url", w.URL, "err", err)
		return res, true
	}

	sample := home
	postURLs, err := s.discoverPosts(ctx, w.URL)
	if err != nil {
		if isCanceled(ctx, err) {
			return domain.Result{}, false
		}
		// Graceful degradation: classify on the home page alone when post
		// discovery fails, but keep the failure distinguishable from "no posts".
		log.DebugContext(ctx, "posts discover failed, using home page only", "url", w.URL, "err", err)
	}
	if len(postURLs) > 0 {
		extras := s.fetchPosts(ctx, log, w.URL, postURLs)
		sample = mergePages(home, extras)
	}

	res.OutboundDomains = domain.CountExternalDomains(w.URL, sample.Links)
	res.PostsSampled = len(postURLs)

	topic, err := classifier.Classify(ctx, sample, candidates)
	if err != nil {
		if isCanceled(ctx, err) {
			return domain.Result{}, false
		}
		log.WarnContext(ctx, "classify failed", "url", w.URL, "err", err)
	} else {
		res.Topic = topic
	}
	res.Suitable = domain.IsSuitable(res.Topic, accepted)

	log.InfoContext(ctx, "site qualified",
		"url", w.URL,
		"topic", res.Topic,
		"outbound", res.OutboundDomains,
		"posts_sampled", res.PostsSampled,
		"merged_text_chars", len(sample.TextSample),
		"links_total", len(sample.Links),
		"suitable", res.Suitable,
	)
	return res, true
}

func (s *Service) discoverPosts(ctx context.Context, siteURL string) ([]string, error) {
	if s.posts == nil {
		return nil, nil
	}
	dCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	urls, err := s.posts.Discover(dCtx, siteURL, maxPostsPerSite)
	if err != nil {
		return nil, fmt.Errorf("discover posts: %w", err)
	}
	return urls, nil
}

func (s *Service) fetchPosts(ctx context.Context, log *slog.Logger, siteURL string, urls []string) []domain.Page {
	if len(urls) == 0 {
		return nil
	}
	out := make([]domain.Page, 0, len(urls))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(postURL string) {
			defer wg.Done()
			p, err := s.fetcher.Fetch(ctx, postURL)
			if err != nil {
				log.DebugContext(ctx, "post fetch failed", "site", siteURL, "post", postURL, "err", err)
				return
			}
			mu.Lock()
			out = append(out, p)
			mu.Unlock()
		}(u)
	}
	wg.Wait()
	return out
}

func mergePages(home domain.Page, extras []domain.Page) domain.Page {
	out := home
	out.TextSample = trimRunes(out.TextSample, perPageChars)
	for _, p := range extras {
		snippet := trimRunes(p.TextSample, perPageChars)
		if snippet != "" {
			if out.TextSample != "" {
				out.TextSample += "\n\n"
			}
			out.TextSample += snippet
		}
		out.Headings = append(out.Headings, p.Headings...)
		out.Links = append(out.Links, p.Links...)
	}
	return out
}

func trimRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

func isCanceled(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
