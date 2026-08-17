package linkbuilding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/url"
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
	creds          domain.CredentialSource
	placements     domain.PlacementSink
	store          domain.PlacementStore
	profiles       domain.DonorProfileStore
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
	lockedCooldown time.Duration
	failCooldown   time.Duration
	cancels        sync.Map
}

type BacklinkOption func(*BacklinkService)

func WithBacklinkDelay(min, max time.Duration) BacklinkOption {
	return func(s *BacklinkService) { s.minDelay, s.maxDelay = min, max }
}

func WithCooldown(locked, fail time.Duration) BacklinkOption {
	return func(s *BacklinkService) { s.lockedCooldown, s.failCooldown = locked, fail }
}

func NewBacklinkService(
	creds domain.CredentialSource,
	placements domain.PlacementSink,
	store domain.PlacementStore,
	profiles domain.DonorProfileStore,
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
		creds:          creds,
		placements:     placements,
		store:          store,
		profiles:       profiles,
		donors:         donors,
		issuer:         issuer,
		editor:         editor,
		placerBuilder:  placerBuilder,
		defaults:       defaults,
		targets:        targets,
		runner:         runner,
		log:            log,
		minDelay:       2 * time.Second,
		maxDelay:       5 * time.Second,
		lockedCooldown: 24 * time.Hour,
		failCooldown:   6 * time.Hour,
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

	now := time.Now()
	cooling, err := s.profiles.RecentlyFailed(ctx, now.Add(-s.lockedCooldown), now.Add(-s.failCooldown))
	if err != nil {
		return PlaceBacklinksQueued{}, fmt.Errorf("list recently failed donors: %w", err)
	}

	queued := make([]domain.SiteCredential, 0, len(creds))
	var skippedPlaced, skippedBlocked, skippedCooldown int
	for _, c := range creds {
		key := domain.NormalizeDonorURL(c.BaseURL)
		if placed[key] || hasLatestPlaced(c.PlacementStatus) {
			skippedPlaced++
			continue
		}
		if domain.IsPermanentStatus(c.LoginStatus) {
			skippedBlocked++
			continue
		}
		if cooling[key] {
			skippedCooldown++
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
		"skipped_cooldown", skippedCooldown,
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
	dbSaveFailures := 0

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

		res, caps := s.placeOne(ctx, log, c, targetURL, placer)

		if ctx.Err() != nil {
			log.WarnContext(ctx, "backlink run cancelled mid-step", "url", c.BaseURL, "processed", processed)
			break
		}

		processed++
		pending = append(pending, res)
		if res.OK {
			succeeded++
		}
		if err := s.savePlacement(ctx, log, runID, sheet, targetURL, res); err != nil {
			dbSaveFailures++
		}
		s.saveProfile(ctx, log, c.BaseURL, caps, res.Outcome)
		log.InfoContext(ctx, "donor placement", "row", c.Row, "url", c.BaseURL, "ok", res.OK, "link_check", res.LinkCheck, "status", res.Status)

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
	log.InfoContext(ctx, "backlink placement done", "processed", processed, "succeeded", succeeded, "db_save_failures", dbSaveFailures)
}

// savePlacement persists the per-donor placement record to Postgres. This is
// separate from the batched Sheets write in flush() above — a failure here
// means the placement won't show up in ListPlacements/ListPlaced even though
// the sheet and the site itself may be fine, so the caller counts failures
// instead of losing the signal silently.
func (s *BacklinkService) savePlacement(ctx context.Context, log *slog.Logger, runID, sheet, targetURL string, res domain.PlacementResult) error {
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := s.store.Save(saveCtx, domain.Placement{
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
		LinkCheck: res.LinkCheck,
	})
	if err != nil {
		log.ErrorContext(ctx, "save placement record failed", "url", res.DonorURL, "err", err)
	}
	return err
}

func (s *BacklinkService) verifyLink(ctx context.Context, log *slog.Logger, pageURL, targetURL string) string {
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	check, err := s.editor.VerifyLink(checkCtx, pageURL, targetURL)
	if err != nil {
		log.WarnContext(ctx, "link check failed", "url", pageURL, "reason", sanitizeReason(err.Error()))
		return ""
	}
	return check
}

func (s *BacklinkService) saveProfile(ctx context.Context, log *slog.Logger, donorURL string, caps domain.DonorCapabilities, outcome string) {
	if s.profiles == nil {
		return
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.profiles.Save(saveCtx, domain.DonorProfile{
		DonorURL:      donorURL,
		Role:          strings.Join(caps.Roles, ","),
		CanEditPages:  caps.CanEditPages,
		CanEditOthers: caps.CanEditOthers,
		CanPublish:    caps.CanPublish,
		CanCreate:     caps.CanCreate,
		LastOutcome:   outcome,
	}); err != nil {
		log.WarnContext(ctx, "save donor profile failed", "url", donorURL, "err", err)
	}
}

func (s *BacklinkService) placeOne(ctx context.Context, log *slog.Logger, c domain.SiteCredential, targetURL string, placer domain.BacklinkPlacer) (domain.PlacementResult, domain.DonorCapabilities) {
	res := domain.PlacementResult{Row: c.Row, DonorURL: c.BaseURL}

	donor, ok, err := s.donors.Get(ctx, c.BaseURL)
	if err != nil {
		reason := sanitizeReason(err.Error())
		log.WarnContext(ctx, "donor stage failed", "stage", "load app password", "url", c.BaseURL, "reason", reason)
		res.Outcome = domain.OutcomeError
		res.Status = "failed: load app password: " + reason
		return res, domain.DonorCapabilities{}
	}
	if !ok {
		appPwd, err := s.issuer.IssueAppPassword(ctx, c.BaseURL, c.Login, c.Password)
		if err != nil {
			reason := sanitizeReason(err.Error())
			res.Outcome = domain.ClassifyLoginOutcome(reason)
			log.WarnContext(ctx, "donor stage failed", "stage", "issue app password", "url", c.BaseURL, "outcome", res.Outcome, "reason", reason)
			res.Status = "failed: issue app password: " + reason
			return res, domain.DonorCapabilities{}
		}
		donor = domain.DonorCredential{DonorURL: c.BaseURL, Login: c.Login, AppPassword: appPwd}
		if err := s.donors.Save(ctx, donor); err != nil {
			reason := sanitizeReason(err.Error())
			log.ErrorContext(ctx, "donor stage failed", "stage", "persist credential", "url", c.BaseURL, "reason", reason)
			res.Outcome = domain.OutcomeError
			res.Status = "failed: persist credential: " + reason
			return res, domain.DonorCapabilities{}
		}
	}

	return s.placeOnDonor(ctx, log, res, donor, targetURL, placer)
}

func (s *BacklinkService) placeOnDonor(ctx context.Context, log *slog.Logger, res domain.PlacementResult, donor domain.DonorCredential, targetURL string, placer domain.BacklinkPlacer) (domain.PlacementResult, domain.DonorCapabilities) {
	url := res.DonorURL
	var hardErr string
	var sawMissing bool
	var notes []string
	note := func(s string) { notes = append(notes, s) }
	fail := func(tier, stage, reason string) {
		log.WarnContext(ctx, "tier failed", "tier", tier, "stage", stage, "url", url, "reason", reason)
		note(tier + ": " + reason)
		if !isPermissionReason(reason) {
			hardErr = reason
		}
	}

	caps, err := s.editor.Capabilities(ctx, donor)
	if err != nil {
		log.WarnContext(ctx, "donor capabilities unknown", "url", url, "reason", sanitizeReason(err.Error()))
		caps = domain.DonorCapabilities{CanEditPages: true, CanEditOthers: true, CanPublish: true, CanCreate: true}
		res.Rights = "unknown"
	} else {
		res.Rights = domain.RightsSummary(caps)
		log.InfoContext(ctx, "donor capabilities", "url", url, "rights", res.Rights)
	}

	anchor := anchorFromURL(targetURL)
	touched := false

	switch {
	case !caps.CanEditPages:
		note("homepage: no edit-pages rights")
	default:
		touched = true
		if page, ok, err := s.editor.FrontPage(ctx, donor); err != nil {
			fail("homepage", "front page", sanitizeReason(err.Error()))
		} else if !ok {
			note("homepage: no static homepage")
		} else if err := s.editor.UpdatePageContent(ctx, donor, page.ID, page.Content+hiddenLink(targetURL, anchor)); err != nil {
			fail("homepage", "update", sanitizeReason(err.Error()))
		} else if r, ok := s.acceptIfLive(ctx, log, res, "homepage", page.PublicURL, page.EditURL, anchor, targetURL); ok {
			return r, caps
		} else {
			sawMissing = true
			note("homepage: link not visible")
		}
	}

	if touched {
		s.sleep(ctx)
		touched = false
	}

	switch {
	case !caps.CanCreate && !caps.CanEditOthers:
		note("post: no edit rights")
	default:
		touched = true
		if post, ok, err := s.editor.LatestEditablePost(ctx, donor, caps); err != nil {
			fail("post", "find post", sanitizeReason(err.Error()))
		} else if !ok {
			note("post: no editable post")
		} else if ins, err := placer.Place(ctx, post.Content, targetURL); err != nil {
			fail("post", "llm", sanitizeReason(err.Error()))
		} else if err := s.editor.UpdatePostContent(ctx, donor, post.ID, ins.ModifiedHTML); err != nil {
			fail("post", "update", sanitizeReason(err.Error()))
		} else if r, ok := s.acceptIfLive(ctx, log, res, "post", post.PublicURL, post.EditURL, ins.Anchor, targetURL); ok {
			return r, caps
		} else {
			sawMissing = true
			note("post: link not visible")
		}
	}

	if touched {
		s.sleep(ctx)
		touched = false
	}

	switch {
	case !caps.CanCreate:
		note("new_post: cannot create (no rights)")
	default:
		titles, terr := s.editor.LatestTitles(ctx, donor, 3)
		if terr != nil {
			log.WarnContext(ctx, "latest titles failed", "url", url, "reason", sanitizeReason(terr.Error()))
		}
		if composed, err := placer.Compose(ctx, targetURL, titles); err != nil {
			fail("new_post", "llm", sanitizeReason(err.Error()))
		} else if created, err := s.editor.CreatePost(ctx, donor, composed.Title, composed.HTML); err != nil {
			fail("new_post", "create", sanitizeReason(err.Error()))
		} else if created.Status != "publish" {
			res.Outcome = domain.OutcomePending
			res.PostURL = created.PublicURL
			res.EditURL = created.EditURL
			res.Anchor = composed.Anchor
			res.Status = fmt.Sprintf("pending review (not live, %s) — edit: %s | preview: %s | view: %s", created.Status, created.EditURL, created.PreviewURL, created.PublicURL)
			return res, caps
		} else if r, ok := s.acceptIfLive(ctx, log, res, "new_post", created.PublicURL, created.EditURL, composed.Anchor, targetURL); ok {
			return r, caps
		} else {
			sawMissing = true
			note("new_post: link not visible")
		}
	}

	switch {
	case hardErr != "":
		res.Outcome = domain.OutcomePostFailed
	case sawMissing:
		res.Outcome = domain.OutcomeLinkMissing
	default:
		res.Outcome = domain.OutcomeNoTarget
	}
	res.Status = "no live placement: " + strings.Join(notes, " | ")
	return res, caps
}

func (s *BacklinkService) acceptIfLive(ctx context.Context, log *slog.Logger, res domain.PlacementResult, method, publicURL, editURL, anchor, targetURL string) (domain.PlacementResult, bool) {
	check := s.verifyLink(ctx, log, publicURL, targetURL)
	if check == domain.LinkMissing {
		log.WarnContext(ctx, "placement not live, trying next tier", "tier", method, "url", res.DonorURL)
		return res, false
	}
	res.LinkCheck = check
	return placed(res, method, publicURL, editURL, anchor), true
}

func placed(res domain.PlacementResult, method, publicURL, editURL, anchor string) domain.PlacementResult {
	res.OK = true
	res.Outcome = domain.OutcomePlaced
	res.PostURL = publicURL
	res.EditURL = editURL
	res.Anchor = anchor
	res.Status = fmt.Sprintf("placed (%s): %s | edit: %s (anchor: %s)", method, publicURL, editURL, anchor)
	return res
}

func hiddenLink(targetURL, anchor string) string {
	return fmt.Sprintf(`<div style="display:none"><a href="%s" rel="noopener">%s</a></div>`, targetURL, anchor)
}

func anchorFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "link"
	}
	host := strings.TrimPrefix(u.Host, "www.")
	if i := strings.IndexByte(host, '.'); i > 0 {
		host = host[:i]
	}
	if host == "" {
		return "link"
	}
	return host
}

func isPermissionReason(reason string) bool {
	low := strings.ToLower(reason)
	return strings.Contains(low, "401") ||
		strings.Contains(low, "403") ||
		strings.Contains(low, "cannot_") ||
		strings.Contains(low, "not allowed") ||
		strings.Contains(low, "no editable")
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
