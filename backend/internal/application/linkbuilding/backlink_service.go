package linkbuilding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	domain "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/jobrunner"
)

const placeWriteChunk = 5

var ErrNoTargetSite = errors.New("target site id is required")

type TargetSiteResolver interface {
	ResolveSiteURL(ctx context.Context, url string) (string, error)
}

type BacklinkPlacerBuilder func(provider, model string) (domain.BacklinkPlacer, error)

type BacklinkService struct {
	creds         domain.CredentialSource
	placements    domain.PlacementSink
	store         domain.PlacementStore
	donors        domain.DonorCredentialStore
	issuer        domain.DonorAppPasswordIssuer
	editor        domain.DonorPostEditor
	placerBuilder BacklinkPlacerBuilder
	defaults      LLMDefaults
	targets       TargetSiteResolver
	runner        jobrunner.JobRunner
	log           *slog.Logger
	minDelay      time.Duration
	maxDelay      time.Duration
	cancels       sync.Map
}

type BacklinkOption func(*BacklinkService)

func WithBacklinkDelay(min, max time.Duration) BacklinkOption {
	return func(s *BacklinkService) { s.minDelay, s.maxDelay = min, max }
}

func NewBacklinkService(
	creds domain.CredentialSource,
	placements domain.PlacementSink,
	store domain.PlacementStore,
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
		store:         store,
		donors:        donors,
		issuer:        issuer,
		editor:        editor,
		placerBuilder: placerBuilder,
		defaults:      defaults,
		targets:       targets,
		runner:        runner,
		log:           log,
		minDelay:      2 * time.Second,
		maxDelay:      5 * time.Second,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

type PlaceBacklinksRequest struct {
	Sheet         string
	TargetSiteURL string
	Count         int
	Provider      string
	Model         string
}

type PlaceBacklinksQueued struct {
	Sheet       string
	SitesQueued int
	RunID       string
}

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

	placed, err := s.store.PlacedDonors(ctx, targetURL)
	if err != nil {
		return PlaceBacklinksQueued{}, fmt.Errorf("list placed donors: %w", err)
	}

	queued := make([]domain.SiteCredential, 0, len(creds))
	var skippedPlaced, skippedBlocked int
	for _, c := range creds {
		if placed[domain.NormalizeDonorURL(c.BaseURL)] || hasLatestPlaced(c.PlacementStatus) {
			skippedPlaced++
			continue
		}
		if domain.IsPermanentStatus(c.LoginStatus) {
			skippedBlocked++
			continue
		}
		queued = append(queued, c)
	}

	provider := pickStr(req.Provider, s.defaults.Provider)
	model := req.Model
	if model == "" && provider == s.defaults.Provider {
		model = s.defaults.Model
	}
	placer, err := s.placerBuilder(provider, model)
	if err != nil {
		return PlaceBacklinksQueued{}, fmt.Errorf("build placer (%s/%s): %w", provider, model, err)
	}

	target := req.Count
	if target <= 0 {
		target = 3
	}
	runID := uuid.New().String()

	jobLog := s.log.With(
		"sheet", req.Sheet,
		"sites", len(queued),
		"success_target", target,
		"run_id", runID,
		"skipped_placed", skippedPlaced,
		"skipped_blocked", skippedBlocked,
		"target_url", targetURL,
		"provider", provider,
		"model", model,
	)
	s.runner.Go(ctx, func(bg context.Context) {
		runCtx, cancel := context.WithCancel(bg)
		s.cancels.Store(runID, cancel)
		defer s.cancels.Delete(runID)
		defer cancel()
		s.placeAll(runCtx, jobLog, runID, req.Sheet, queued, targetURL, placer, target)
	})

	jobLog.InfoContext(ctx, "backlink placement accepted")
	return PlaceBacklinksQueued{Sheet: req.Sheet, SitesQueued: len(queued), RunID: runID}, nil
}

func (s *BacklinkService) ListPlacements(ctx context.Context, runID string) ([]domain.Placement, error) {
	return s.store.ListByRun(ctx, runID)
}

func (s *BacklinkService) ListPlaced(ctx context.Context, limit, offset int) ([]domain.Placement, int, error) {
	return s.store.ListPlaced(ctx, limit, offset)
}

// Cancel stops a placement run executing in this process; a finished or unknown
// run is a no-op.
func (s *BacklinkService) Cancel(_ context.Context, runID string) {
	if c, ok := s.cancels.Load(runID); ok {
		c.(context.CancelFunc)()
	}
}

func (s *BacklinkService) placeAll(ctx context.Context, log *slog.Logger, runID, sheet string, creds []domain.SiteCredential, targetURL string, placer domain.BacklinkPlacer, target int) {
	pending := make([]domain.PlacementResult, 0, placeWriteChunk)
	processed := 0
	succeeded := 0

	flush := func() bool {
		if len(pending) == 0 {
			return true
		}
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		err := s.placements.WritePlacementStatus(writeCtx, sheet, pending)
		cancel()
		if err != nil {
			log.ErrorContext(ctx, "write placement status failed", "reason", sanitizeReason(err.Error()), "unpersisted", len(pending))
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

		log.InfoContext(ctx, "donor selected", "row", c.Row, "url", c.BaseURL, "login_status", c.LoginStatus)

		res := s.placeOne(ctx, log, c, targetURL, placer)

		if ctx.Err() != nil {
			log.WarnContext(ctx, "backlink run cancelled mid-step", "url", c.BaseURL, "processed", processed)
			break
		}

		processed++
		pending = append(pending, res)
		if res.OK {
			succeeded++
		}
		s.savePlacement(ctx, log, runID, sheet, targetURL, res)
		log.InfoContext(ctx, "donor placement", "row", c.Row, "url", c.BaseURL, "ok", res.OK, "status", res.Status)

		if len(pending) >= placeWriteChunk {
			if !flush() {
				return
			}
		}
		if target > 0 && succeeded >= target {
			break
		}
	}

	if !flush() {
		// flush() already logged the write failure with detail; suppress the success
		// "done" log so the run is not reported as complete with results unpersisted.
		return
	}
	log.InfoContext(ctx, "backlink placement done", "processed", processed, "succeeded", succeeded)
}

func (s *BacklinkService) savePlacement(ctx context.Context, log *slog.Logger, runID, sheet, targetURL string, res domain.PlacementResult) {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.store.Save(saveCtx, domain.Placement{
		RunID:     runID,
		Sheet:     sheet,
		DonorURL:  res.DonorURL,
		TargetURL: targetURL,
		OK:        res.OK,
		Outcome:   res.Outcome,
		Status:    res.Status,
		PostURL:   res.PostURL,
		EditURL:   res.EditURL,
		Anchor:    res.Anchor,
	}); err != nil {
		log.WarnContext(ctx, "save placement record failed", "url", res.DonorURL, "err", err)
	}
}

func (s *BacklinkService) placeOne(ctx context.Context, log *slog.Logger, c domain.SiteCredential, targetURL string, placer domain.BacklinkPlacer) domain.PlacementResult {
	res := domain.PlacementResult{Row: c.Row, DonorURL: c.BaseURL}

	donor, ok, err := s.donors.Get(ctx, c.BaseURL)
	if err != nil {
		reason := sanitizeReason(err.Error())
		log.WarnContext(ctx, "donor stage failed", "stage", "load app password", "url", c.BaseURL, "reason", reason)
		res.Outcome = domain.OutcomeError
		res.Status = "failed: load app password: " + reason
		return res
	}
	if !ok {
		appPwd, err := s.issuer.IssueAppPassword(ctx, c.BaseURL, c.Login, c.Password)
		if err != nil {
			reason := sanitizeReason(err.Error())
			res.Outcome = domain.ClassifyLoginOutcome(reason)
			log.WarnContext(ctx, "donor stage failed", "stage", "issue app password", "url", c.BaseURL, "outcome", res.Outcome, "reason", reason)
			res.Status = "failed: issue app password: " + reason
			return res
		}
		donor = domain.DonorCredential{DonorURL: c.BaseURL, Login: c.Login, AppPassword: appPwd}
		if err := s.donors.Save(ctx, donor); err != nil {
			reason := sanitizeReason(err.Error())
			log.ErrorContext(ctx, "donor stage failed", "stage", "persist credential", "url", c.BaseURL, "reason", reason)
			res.Outcome = domain.OutcomeError
			res.Status = "failed: persist credential: " + reason
			return res
		}
	}

	post, err := s.editor.LatestPost(ctx, donor)
	if err != nil {
		reason := sanitizeReason(err.Error())
		log.WarnContext(ctx, "donor stage failed", "stage", "latest post", "url", c.BaseURL, "reason", reason)
		res.Outcome = domain.OutcomePostFailed
		res.Status = "failed: latest post: " + reason
		return res
	}

	ins, err := placer.Place(ctx, post.Content, targetURL)
	if err != nil {
		reason := sanitizeReason(err.Error())
		log.WarnContext(ctx, "donor stage failed", "stage", "llm", "url", c.BaseURL, "reason", reason)
		res.Outcome = domain.OutcomePostFailed
		res.Status = "failed: llm: " + reason
		return res
	}

	if err := s.editor.UpdatePostContent(ctx, donor, post.ID, ins.ModifiedHTML); err != nil {
		reason := sanitizeReason(err.Error())
		log.WarnContext(ctx, "donor stage failed", "stage", "update post", "url", c.BaseURL, "reason", reason)
		res.Outcome = domain.OutcomePostFailed
		res.Status = "failed: update post: " + reason
		return res
	}

	res.OK = true
	res.Outcome = domain.OutcomePlaced
	res.PostURL = post.PublicURL
	res.EditURL = post.EditURL
	res.Anchor = ins.Anchor
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

var (
	// HTTP response snippets embedded by lower layers (e.g. issuer/editor) can carry
	// auth/captcha detail; drop everything from the snippet marker onward.
	bodySnippetRe = regexp.MustCompile(`(?is)\b(body|response|snippet)\b.*`)
	// Common credential-bearing query/JSON fragments.
	credPatternRe = regexp.MustCompile(`(?i)(app[_-]?password|password|secret|token|authorization|api[_-]?key)\s*[=:]\s*\S+`)
)

// sanitizeReason strips HTTP response bodies and credential-bearing fragments out
// of an error string before it is logged or persisted to the sheet (res.Status).
func sanitizeReason(s string) string {
	s = bodySnippetRe.ReplaceAllString(s, "[redacted body]")
	s = credPatternRe.ReplaceAllString(s, "[redacted credential]")
	s = strings.TrimSpace(s)
	const max = 120
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func hasLatestPlaced(status string) bool {
	first := strings.TrimSpace(status)
	if idx := strings.IndexByte(first, '\n'); idx >= 0 {
		first = first[:idx]
	}
	low := strings.ToLower(first)
	return strings.Contains(low, "] placed:") || strings.HasPrefix(low, "placed:")
}
