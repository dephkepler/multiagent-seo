package emailscrape

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domain "multiagent-seo/internal/domain/emailscrape"
	"multiagent-seo/pkg/hostlimit"
	"multiagent-seo/pkg/jobrunner"
	"multiagent-seo/pkg/workpool"
)

const (
	maxConcurrentSites = 15
	resultFlushBatch   = 50
	resultWriteTimeout = 30 * time.Second
	maxContactPages    = 8
	perHostRPS         = 1.0
	perHostBurst       = 1
	maxStatusReason    = 200
)

var ErrNoSheet = errors.New("sheet name is required")

var contactKeywords = []string{
	"contact", "kontakt", "kontakty", "contato", "contacto", "contatti", "contacter",
	"about", "über", "uber", "impressum", "imprint", "legal", "mentions", "aviso",
	"privacy", "datenschutz",
	// visible-text keywords (matched against anchor text, often non-latin)
	"контакт", "связ", "о нас", "о компании", "реквизит",
}

type Service struct {
	sites   domain.SiteSource
	fetcher domain.PageFetcher
	store   domain.EmailStore
	jobs    domain.JobStore
	runner  jobrunner.JobRunner
	log     *slog.Logger
	cancels sync.Map // jobID -> context.CancelFunc, for jobs running in this process
}

func NewService(sites domain.SiteSource, fetcher domain.PageFetcher, store domain.EmailStore, jobs domain.JobStore, runner jobrunner.JobRunner, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{sites: sites, fetcher: fetcher, store: store, jobs: jobs, runner: runner, log: log}
}

type ScrapeRequest struct {
	Sheet string
}

type ScrapeResult struct {
	Sheet       string
	SitesQueued int
	JobID       string
}

func (s *Service) ScrapeEmails(ctx context.Context, req ScrapeRequest) (ScrapeResult, error) {
	if req.Sheet == "" {
		return ScrapeResult{}, ErrNoSheet
	}

	sites, err := s.sites.List(ctx, req.Sheet)
	if err != nil {
		return ScrapeResult{}, fmt.Errorf("list sites: %w", err)
	}
	if len(sites) == 0 {
		return ScrapeResult{Sheet: req.Sheet}, nil
	}

	jobID, err := s.jobs.Create(ctx, req.Sheet, len(sites))
	if err != nil {
		return ScrapeResult{}, fmt.Errorf("create job: %w", err)
	}

	jobLog := s.log.With("sheet", req.Sheet, "sites", len(sites), "job_id", jobID)
	limiter := hostlimit.New(perHostRPS, perHostBurst)
	s.runner.Go(ctx, func(bg context.Context) {
		jobCtx, cancel := context.WithCancel(bg)
		s.cancels.Store(jobID, cancel)
		defer s.cancels.Delete(jobID)
		defer cancel()
		s.scrapeAll(jobCtx, jobLog, jobID, req.Sheet, sites, limiter)
	})

	jobLog.InfoContext(ctx, "email scrape accepted")
	return ScrapeResult{Sheet: req.Sheet, SitesQueued: len(sites), JobID: jobID}, nil
}

// JobStatus returns the persisted progress/state of a scrape job.
func (s *Service) JobStatus(ctx context.Context, id string) (domain.Job, error) {
	return s.jobs.Get(ctx, id)
}

// Cancel stops a job running in this process; if it's not running, it just
// confirms the job exists (a finished or unknown job).
func (s *Service) Cancel(ctx context.Context, id string) error {
	if c, ok := s.cancels.Load(id); ok {
		c.(context.CancelFunc)()
		return nil
	}
	if _, err := s.jobs.Get(ctx, id); err != nil {
		return err
	}
	return nil
}

func (s *Service) scrapeAll(ctx context.Context, log *slog.Logger, jobID, sheet string, sites []domain.Site, limiter *hostlimit.Limiter) {
	var done atomic.Int64
	processed, err := workpool.Run(ctx, sites,
		func(ctx context.Context, site domain.Site) (domain.Result, bool) {
			res := s.scrapeOne(ctx, log, site, limiter)
			if ctx.Err() != nil {
				return domain.Result{}, false // don't mark a site done if we were canceled mid-flight
			}
			return res, true
		},
		func(ctx context.Context, batch []domain.Result) error {
			if err := s.writeBatch(ctx, log, sheet, batch); err != nil {
				return err
			}
			n := int(done.Add(int64(len(batch))))
			if err := s.jobs.UpdateProgress(ctx, jobID, n); err != nil {
				log.WarnContext(ctx, "update job progress failed", "err", err)
			}
			log.InfoContext(ctx, "email scrape progress", "processed", n, "total", len(sites))
			return nil
		},
		workpool.Options{Concurrency: maxConcurrentSites, FlushEvery: resultFlushBatch, FlushTimeout: resultWriteTimeout},
	)

	state, errMsg := domain.JobDone, ""
	switch {
	case err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)):
		state, errMsg = domain.JobCanceled, err.Error()
		log.WarnContext(ctx, "email scrape aborted", "processed", processed, "total", len(sites), "err", err)
	case err != nil:
		state, errMsg = domain.JobFailed, err.Error()
		log.ErrorContext(ctx, "email scrape stopped on write failure", "processed", processed, "total", len(sites), "err", err)
	default:
		log.InfoContext(ctx, "email scrape done", "processed", processed)
	}

	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resultWriteTimeout)
	defer cancel()
	if err := s.jobs.Finish(finishCtx, jobID, state, errMsg); err != nil {
		log.ErrorContext(ctx, "finish job failed", "job_id", jobID, "err", err)
	}
}

func (s *Service) scrapeOne(ctx context.Context, log *slog.Logger, site domain.Site, limiter *hostlimit.Limiter) domain.Result {
	res := domain.Result{Row: site.Row, URL: site.URL}

	home, err := s.fetchWithLimit(ctx, limiter, site.URL)
	if err != nil {
		res.Status = "error: " + shortReason(err)
		return res
	}

	found := make(map[string]struct{})
	for _, e := range domain.ExtractEmails(home.HTML) {
		found[e] = struct{}{}
	}

	for _, link := range pickContactLinks(home, maxContactPages) {
		if ctx.Err() != nil {
			break
		}
		page, err := s.fetchWithLimit(ctx, limiter, link)
		if err != nil {
			log.DebugContext(ctx, "contact page fetch failed", "url", link, "err", err)
			continue
		}
		for _, e := range domain.ExtractEmails(page.HTML) {
			found[e] = struct{}{}
		}
	}

	res.Emails = sortedKeys(found)
	if len(res.Emails) == 0 {
		res.Status = "no emails"
	} else {
		res.Status = fmt.Sprintf("found %d", len(res.Emails))
	}
	return res
}

func (s *Service) writeBatch(ctx context.Context, log *slog.Logger, sheet string, batch []domain.Result) error {
	if err := s.sites.WriteResults(ctx, sheet, batch); err != nil {
		return fmt.Errorf("write results: %w", err)
	}

	var records []domain.EmailRecord
	for _, r := range batch {
		for _, e := range r.Emails {
			records = append(records, domain.EmailRecord{ListName: sheet, SiteURL: r.URL, Email: e})
		}
	}
	if err := s.store.Save(ctx, records); err != nil {
		// The sheet already has the emails; a DB hiccup shouldn't abort the crawl.
		log.WarnContext(ctx, "save emails to db failed", "records", len(records), "err", err)
	}
	return nil
}

func (s *Service) fetchWithLimit(ctx context.Context, limiter *hostlimit.Limiter, rawURL string) (domain.Page, error) {
	if err := limiter.Wait(ctx, hostOf(rawURL)); err != nil {
		return domain.Page{}, err
	}
	return s.fetcher.Fetch(ctx, rawURL)
}

func pickContactLinks(page domain.Page, max int) []string {
	base, err := url.Parse(page.URL)
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, link := range page.Links {
		if !containsAny(strings.ToLower(link.Href), contactKeywords) && !containsAny(strings.ToLower(link.Text), contactKeywords) {
			continue
		}
		ref, err := url.Parse(link.Href)
		if err != nil {
			continue
		}
		abs := base.ResolveReference(ref)
		if abs.Scheme != "http" && abs.Scheme != "https" {
			continue
		}
		u := abs.String()
		if u == page.URL {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
		if len(out) >= max {
			break
		}
	}
	return out
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.Host
	}
	return ""
}

func shortReason(err error) string {
	r := err.Error()
	if len(r) > maxStatusReason {
		r = r[:maxStatusReason] + "…"
	}
	return r
}
