package linkbuilding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	domain "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/jobrunner"
)

// placeWriteChunk is how many placement statuses we flush at once. WP REST
// edits are slower than wp-login form posts, so the chunk is smaller than in
// LoginService — partial progress still survives a job timeout.
const placeWriteChunk = 5

// ErrNoTargetSite guards the request; the HTTP layer maps it to 400.
var ErrNoTargetSite = errors.New("target site id is required")

// TargetSiteResolver confirms the per-request target_site_url belongs to one
// of our wordpress_sites and returns the canonical (stored) form of that URL,
// which is what we plant into donor posts.
type TargetSiteResolver interface {
	ResolveSiteURL(ctx context.Context, url string) (string, error)
}

// BacklinkPlacerBuilder constructs a fresh placer bound to the given
// provider/model. Mirrors TopicClassifierBuilder so per-request overrides flow
// through the same path as deployment-wide defaults.
type BacklinkPlacerBuilder func(provider, model string) (domain.BacklinkPlacer, error)

// BacklinkService runs Flow 3: for each suitable donor in the sheet, ensure we
// have a stored app-password (issue + save on first contact), fetch the
// latest post via REST, let an LLM weave in a backlink to the client's site,
// PUT the modified HTML back, and record the result in the sheet's I column.
type BacklinkService struct {
	creds          domain.CredentialSource
	placements     domain.PlacementSink
	donors         domain.DonorCredentialStore
	issuer         domain.DonorAppPasswordIssuer
	editor         domain.DonorPostEditor
	placerBuilder  BacklinkPlacerBuilder
	defaults       LLMDefaults
	targets        TargetSiteResolver
	runner         jobrunner.JobRunner
	log            *slog.Logger
	minDelay       time.Duration
	maxDelay       time.Duration
}

// BacklinkOption customizes a BacklinkService; WithBacklinkDelay disables the
// inter-donor pause in tests.
type BacklinkOption func(*BacklinkService)

func WithBacklinkDelay(min, max time.Duration) BacklinkOption {
	return func(s *BacklinkService) { s.minDelay, s.maxDelay = min, max }
}

func NewBacklinkService(
	creds domain.CredentialSource,
	placements domain.PlacementSink,
	donors domain.DonorCredentialStore,
	issuer domain.DonorAppPasswordIssuer,
	editor domain.DonorPostEditor,
	placerBuilder BacklinkPlacerBuilder,
	defaults LLMDefaults,
	targets TargetSiteResolver,
	runner jobrunner.JobRunner,
	log *slog.Logger,
	opts ...BacklinkOption,
) *BacklinkService {
	if log == nil {
		log = slog.Default()
	}
	s := &BacklinkService{
		creds:         creds,
		placements:    placements,
		donors:        donors,
		issuer:        issuer,
		editor:        editor,
		placerBuilder: placerBuilder,
		defaults:      defaults,
		targets:       targets,
		runner:        runner,
		log:           log,
		// REST edits are heavier than form logins; pause a bit more to stay
		// under any "too many edits" rate limit a security plugin might apply.
		minDelay: 2 * time.Second,
		maxDelay: 5 * time.Second,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

type PlaceBacklinksRequest struct {
	Sheet         string
	TargetSiteURL string
	// Optional campaign-segment filter. When set, only donors whose Flow 1
	// topic (column B) matches one of these (case-insensitive) are processed.
	// Empty slice = no extra filter, all D=yes donors qualify.
	Topics []string
	// Optional LLM overrides for this run; empty falls back to LLMDefaults.
	Provider string
	Model    string
}

type PlaceBacklinksQueued struct {
	Sheet       string
	SitesQueued int
}

// PlaceBacklinks reads the suitable donors and dispatches the placement
// pipeline through the JobRunner, returning immediately (202-style).
func (s *BacklinkService) PlaceBacklinks(ctx context.Context, req PlaceBacklinksRequest) (PlaceBacklinksQueued, error) {
	if req.Sheet == "" {
		return PlaceBacklinksQueued{}, ErrNoSheet
	}
	if req.TargetSiteURL == "" {
		return PlaceBacklinksQueued{}, ErrNoTargetSite
	}

	targetURL, err := s.targets.ResolveSiteURL(ctx, req.TargetSiteURL)
	if err != nil {
		return PlaceBacklinksQueued{}, fmt.Errorf("resolve target site: %w", err)
	}

	creds, err := s.creds.ListCredentials(ctx, req.Sheet)
	if err != nil {
		return PlaceBacklinksQueued{}, fmt.Errorf("list credentials: %w", err)
	}

	// Build a topic allow-set once. Empty set = topic filter disabled.
	topicAllow := make(map[string]struct{}, len(req.Topics))
	for _, t := range req.Topics {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" {
			topicAllow[t] = struct{}{}
		}
	}

	// Three filters:
	// 1. Topic — narrow to current campaign segment (defends against a stale
	//    D=yes lingering from a previous campaign's qualify run).
	// 2. Flow 2 login status — skip rows that already proved we can't
	//    authenticate; an empty status means Flow 2 hasn't run, so we still try.
	// 3. Prior placement — if column I already says "placed: ...", we have
	//    already modified that donor's article; running again would add a
	//    second <a> tag to the same post. Skip; the operator can clear I in
	//    the sheet to force a retry.
	queued := make([]domain.SiteCredential, 0, len(creds))
	var skippedTopic, skippedLogin, skippedAlreadyPlaced int
	for _, c := range creds {
		if len(topicAllow) > 0 {
			if _, ok := topicAllow[strings.TrimSpace(strings.ToLower(c.Topic))]; !ok {
				skippedTopic++
				continue
			}
		}
		st := strings.TrimSpace(strings.ToLower(c.LoginStatus))
		if st != "" && !strings.HasPrefix(st, "login ok") {
			skippedLogin++
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(strings.ToLower(c.PlacementStatus)), "placed:") {
			skippedAlreadyPlaced++
			continue
		}
		queued = append(queued, c)
	}

	provider := pickStr(req.Provider, s.defaults.Provider)
	// Only inherit defaults.Model when the provider didn't change — otherwise
	// we'd ship a groq model name to claude (or vice versa). When the provider
	// flips and model is empty, leave it blank and let the builder/factory
	// pick the right per-provider default.
	model := req.Model
	if model == "" && provider == s.defaults.Provider {
		model = s.defaults.Model
	}
	placer, err := s.placerBuilder(provider, model)
	if err != nil {
		return PlaceBacklinksQueued{}, fmt.Errorf("build placer (%s/%s): %w", provider, model, err)
	}

	jobLog := s.log.With(
		"sheet", req.Sheet,
		"sites", len(queued),
		"skipped_topic_mismatch", skippedTopic,
		"skipped_login_failed", skippedLogin,
		"skipped_already_placed", skippedAlreadyPlaced,
		"target_url", targetURL,
		"provider", provider,
		"model", model,
	)
	s.runner.Go(ctx, func(bg context.Context) {
		s.placeAll(bg, jobLog, req.Sheet, queued, targetURL, placer)
	})

	jobLog.InfoContext(ctx, "backlink placement accepted")
	return PlaceBacklinksQueued{Sheet: req.Sheet, SitesQueued: len(queued)}, nil
}

// placeAll walks the donor list sequentially, flushing statuses in chunks. On
// ctx cancellation we abort instead of writing failures for the remaining
// rows — matches the LoginService convention.
func (s *BacklinkService) placeAll(ctx context.Context, log *slog.Logger, sheet string, creds []domain.SiteCredential, targetURL string, placer domain.BacklinkPlacer) {
	pending := make([]domain.PlacementResult, 0, placeWriteChunk)
	processed := 0

	flush := func() bool {
		if len(pending) == 0 {
			return true
		}
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		err := s.placements.WritePlacementStatus(writeCtx, sheet, pending)
		cancel()
		if err != nil {
			log.ErrorContext(ctx, "write placement status failed", "err", err)
			return false
		}
		pending = pending[:0]
		return true
	}

	for i, c := range creds {
		if ctx.Err() != nil {
			log.WarnContext(ctx, "backlink run cancelled", "processed", processed)
			break
		}
		if i > 0 {
			s.sleep(ctx)
		}

		// Explicit "we are about to process this donor" line so the operator
		// can match the queue against the sheet without waiting for the
		// outcome line (which only appears after the LLM + REST calls).
		log.InfoContext(ctx, "donor selected", "row", c.Row, "url", c.BaseURL, "login_status", c.LoginStatus)

		res := s.placeOne(ctx, log, c, targetURL, placer)

		// An error that bubbles up as a context cancellation means the run
		// got cancelled mid-call — abort without recording a false failure.
		if ctx.Err() != nil {
			log.WarnContext(ctx, "backlink run cancelled mid-step", "url", c.BaseURL, "processed", processed)
			break
		}

		processed++
		pending = append(pending, res)
		log.InfoContext(ctx, "donor placement", "row", c.Row, "url", c.BaseURL, "ok", res.OK, "status", res.Status)

		if len(pending) >= placeWriteChunk {
			if !flush() {
				return
			}
		}
	}

	flush()
	log.InfoContext(ctx, "backlink placement done", "processed", processed)
}

// placeOne is the per-donor pipeline: ensure cached creds → latest post →
// LLM rewrite → REST update. Each step that fails turns into a "failed: ..."
// row in the sheet; the next donor still runs.
func (s *BacklinkService) placeOne(ctx context.Context, log *slog.Logger, c domain.SiteCredential, targetURL string, placer domain.BacklinkPlacer) domain.PlacementResult {
	res := domain.PlacementResult{Row: c.Row, DonorURL: c.BaseURL}

	donor, ok, err := s.donors.Get(ctx, c.BaseURL)
	if err != nil {
		log.WarnContext(ctx, "donor stage failed", "stage", "load app password", "url", c.BaseURL, "err", err)
		res.Status = "failed: load app password: " + truncReason(err.Error())
		return res
	}
	if !ok {
		appPwd, err := s.issuer.IssueAppPassword(ctx, c.BaseURL, c.Login, c.Password)
		if err != nil {
			log.WarnContext(ctx, "donor stage failed", "stage", "issue app password", "url", c.BaseURL, "err", err)
			res.Status = "failed: issue app password: " + truncReason(err.Error())
			return res
		}
		donor = domain.DonorCredential{DonorURL: c.BaseURL, Login: c.Login, AppPassword: appPwd}
		// Save is best-effort: even if persistence fails we still proceed with
		// the in-memory credential; the next run will just re-issue.
		if err := s.donors.Save(ctx, donor); err != nil {
			log.WarnContext(ctx, "save donor credential failed", "url", c.BaseURL, "err", err)
		}
	}

	post, err := s.editor.LatestPost(ctx, donor)
	if err != nil {
		log.WarnContext(ctx, "donor stage failed", "stage", "latest post", "url", c.BaseURL, "err", err)
		res.Status = "failed: latest post: " + truncReason(err.Error())
		return res
	}

	ins, err := placer.Place(ctx, post.Content, targetURL)
	if err != nil {
		log.WarnContext(ctx, "donor stage failed", "stage", "llm", "url", c.BaseURL, "err", err)
		res.Status = "failed: llm: " + truncReason(err.Error())
		return res
	}

	if err := s.editor.UpdatePostContent(ctx, donor, post.ID, ins.ModifiedHTML); err != nil {
		log.WarnContext(ctx, "donor stage failed", "stage", "update post", "url", c.BaseURL, "err", err)
		res.Status = "failed: update post: " + truncReason(err.Error())
		return res
	}

	res.OK = true
	// Two URLs in the sheet status: the public permalink (so the user can open
	// the live post in one click and see the inserted backlink in context) and
	// the wp-admin edit URL (for revisiting/reverting). Public goes first since
	// it's the verification step the user reaches for most often.
	res.Status = fmt.Sprintf("placed: %s | edit: %s (anchor: %s)", post.PublicURL, post.EditURL, ins.Anchor)
	return res
}

func (s *BacklinkService) sleep(ctx context.Context) {
	d := s.jitter()
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func (s *BacklinkService) jitter() time.Duration {
	if s.maxDelay <= s.minDelay {
		return s.minDelay
	}
	return s.minDelay + time.Duration(rand.Int63n(int64(s.maxDelay-s.minDelay)))
}

// truncReason keeps the status cell readable in the sheet — full errors go to
// the structured log; the I column just gets a short pointer.
func truncReason(s string) string {
	const max = 120
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
