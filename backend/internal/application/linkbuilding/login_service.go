package linkbuilding

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	domain "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/jobrunner"
)

// loginWriteChunk is how many statuses we flush at once; writing in chunks
// means a job timeout can't discard the whole batch.
const loginWriteChunk = 10

// LoginService logs into the donor-site network: read the credential inventory
// from a sheet, log into each WordPress admin, write the per-site status back.
// Logins are deliberately sequential with a jittered pause — hammering many
// wp-login.php endpoints trips security plugins (Wordfence) and gets us banned.
type LoginService struct {
	creds    domain.CredentialSource
	auth     domain.SiteAuthenticator
	runner   jobrunner.JobRunner
	log      *slog.Logger
	minDelay time.Duration
	maxDelay time.Duration
}

// LoginOption customizes a LoginService; WithLoginDelay is used by tests to
// disable the inter-login pause.
type LoginOption func(*LoginService)

// WithLoginDelay sets the jittered pause between consecutive logins.
func WithLoginDelay(min, max time.Duration) LoginOption {
	return func(s *LoginService) { s.minDelay, s.maxDelay = min, max }
}

func NewLoginService(creds domain.CredentialSource, auth domain.SiteAuthenticator, runner jobrunner.JobRunner, log *slog.Logger, opts ...LoginOption) *LoginService {
	if log == nil {
		log = slog.Default()
	}
	s := &LoginService{
		creds:    creds,
		auth:     auth,
		runner:   runner,
		log:      log,
		minDelay: 1 * time.Second,
		maxDelay: 3 * time.Second,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

type LoginRequest struct {
	Sheet string
}

type LoginQueued struct {
	Sheet       string
	SitesQueued int
}

// LoginToSites reads the credential inventory and dispatches the login pipeline
// through the JobRunner, returning immediately (202-style). A SyncRunner runs it
// inline (test seam).
func (s *LoginService) LoginToSites(ctx context.Context, req LoginRequest) (LoginQueued, error) {
	if req.Sheet == "" {
		return LoginQueued{}, ErrNoSheet
	}

	creds, err := s.creds.ListCredentials(ctx, req.Sheet)
	if err != nil {
		return LoginQueued{}, fmt.Errorf("list credentials: %w", err)
	}

	// Dispatch even when nothing is suitable: the job still clears stale
	// statuses left on rows that are no longer suitable.
	jobLog := s.log.With("sheet", req.Sheet, "sites", len(creds))
	s.runner.Go(ctx, func(bg context.Context) {
		s.loginAll(bg, jobLog, req.Sheet, creds)
	})

	jobLog.InfoContext(ctx, "site login accepted")
	return LoginQueued{Sheet: req.Sheet, SitesQueued: len(creds)}, nil
}

// loginAll logs into each site in turn, flushing statuses in chunks. A non-nil
// error from Login means the context was cancelled → abort the run rather than
// mark the remaining sites failed; statuses gathered so far are still written.
func (s *LoginService) loginAll(ctx context.Context, log *slog.Logger, sheet string, creds []domain.SiteCredential) {
	// Wipe statuses from rows that are no longer suitable, so H reflects only
	// the sites still in scope (non-fatal if it fails).
	if err := s.creds.ClearStaleStatuses(ctx, sheet); err != nil {
		log.WarnContext(ctx, "clear stale statuses failed", "err", err)
	}

	pending := make([]domain.LoginResult, 0, loginWriteChunk)
	processed := 0

	flush := func() bool {
		if len(pending) == 0 {
			return true
		}
		// Detach from the job context so a timeout that just fired still persists
		// the statuses already collected.
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		err := s.creds.WriteLoginStatus(writeCtx, sheet, pending)
		cancel()
		if err != nil {
			log.ErrorContext(ctx, "write login status failed", "err", err)
			return false
		}
		pending = pending[:0]
		return true
	}

	for i, c := range creds {
		if ctx.Err() != nil {
			log.WarnContext(ctx, "login run cancelled", "processed", processed)
			break
		}
		if i > 0 {
			s.sleep(ctx)
		}

		res, err := s.auth.Login(ctx, c)
		if err != nil {
			log.WarnContext(ctx, "login aborted (context)", "url", c.BaseURL, "err", err)
			break
		}

		processed++
		pending = append(pending, res)
		log.DebugContext(ctx, "site login", "url", c.BaseURL, "ok", res.OK, "status", res.Status)

		if len(pending) >= loginWriteChunk {
			if !flush() {
				return
			}
		}
	}

	flush()
	log.InfoContext(ctx, "site login done", "processed", processed)
}

func (s *LoginService) sleep(ctx context.Context) {
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

func (s *LoginService) jitter() time.Duration {
	if s.maxDelay <= s.minDelay {
		return s.minDelay
	}
	return s.minDelay + time.Duration(rand.Int63n(int64(s.maxDelay-s.minDelay)))
}
