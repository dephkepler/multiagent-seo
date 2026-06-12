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

const loginWriteChunk = 10

type LoginService struct {
	creds    domain.CredentialSource
	auth     domain.SiteAuthenticator
	runner   jobrunner.JobRunner
	log      *slog.Logger
	minDelay time.Duration
	maxDelay time.Duration
}

type LoginOption func(*LoginService)

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
	Sheet  string
	Topics []string
}

type LoginQueued struct {
	Sheet       string
	SitesQueued int
}

func (s *LoginService) LoginToSites(ctx context.Context, req LoginRequest) (LoginQueued, error) {
	if req.Sheet == "" {
		return LoginQueued{}, ErrNoSheet
	}

	creds, err := s.creds.ListCredentials(ctx, req.Sheet)
	if err != nil {
		return LoginQueued{}, fmt.Errorf("list credentials: %w", err)
	}

	topicAllow := make(map[string]struct{}, len(req.Topics))
	for _, t := range req.Topics {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" {
			topicAllow[t] = struct{}{}
		}
	}
	queued := creds
	skippedTopic := 0
	if len(topicAllow) > 0 {
		queued = make([]domain.SiteCredential, 0, len(creds))
		for _, c := range creds {
			if _, ok := topicAllow[strings.TrimSpace(strings.ToLower(c.Topic))]; ok {
				queued = append(queued, c)
				continue
			}
			skippedTopic++
		}
	}

	jobLog := s.log.With("sheet", req.Sheet, "sites", len(queued), "skipped_topic_mismatch", skippedTopic)
	s.runner.Go(ctx, func(bg context.Context) {
		s.loginAll(bg, jobLog, req.Sheet, queued)
	})

	jobLog.InfoContext(ctx, "site login accepted")
	return LoginQueued{Sheet: req.Sheet, SitesQueued: len(queued)}, nil
}

func (s *LoginService) loginAll(ctx context.Context, log *slog.Logger, sheet string, creds []domain.SiteCredential) {
	if err := s.creds.ClearStaleStatuses(ctx, sheet); err != nil {
		log.WarnContext(ctx, "clear stale statuses failed", "err", err)
	}

	pending := make([]domain.LoginResult, 0, loginWriteChunk)
	processed := 0

	flush := func() bool {
		if len(pending) == 0 {
			return true
		}
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
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.WarnContext(ctx, "login cancelled", "row", c.Row, "err", err)
			} else {
				log.ErrorContext(ctx, "login failed", "row", c.Row, "err", err)
			}
			break
		}

		processed++
		pending = append(pending, res)
		log.DebugContext(ctx, "site login", "row", c.Row, "ok", res.OK, "status", res.Status)

		if len(pending) >= loginWriteChunk {
			if !flush() {
				return
			}
		}
	}

	if !flush() {
		return
	}
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
