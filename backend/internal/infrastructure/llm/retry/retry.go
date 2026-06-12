package retry

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type Config struct {
	MaxAttempts int
	Backoffs    []time.Duration
}

func Default() Config {
	return Config{
		MaxAttempts: 5,
		Backoffs:    []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second},
	}
}

const maxRetryAfter = 30 * time.Second

type HTTPStatusError interface {
	error
	HTTPStatus() int
}

type RetryAfterError interface {
	RetryAfter() (time.Duration, bool)
}

func retryAfterOf(err error) (time.Duration, bool) {
	var ra RetryAfterError
	if errors.As(err, &ra) {
		return ra.RetryAfter()
	}
	return 0, false
}

func statusOf(err error) int {
	var he HTTPStatusError
	if errors.As(err, &he) {
		return he.HTTPStatus()
	}
	return 0
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	var he HTTPStatusError
	if !errors.As(err, &he) {
		return false
	}
	status := he.HTTPStatus()
	if status == 0 {
		return true
	}
	if status == 429 {
		return true
	}
	return status >= 500 && status < 600
}

func Do(ctx context.Context, cfg Config, log *slog.Logger, provider string, fn func() error) error {
	if log == nil {
		log = slog.Default()
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if attempt > 1 {
			idx := attempt - 2
			if idx >= len(cfg.Backoffs) {
				idx = len(cfg.Backoffs) - 1
			}
			delay := cfg.Backoffs[idx]
			if ra, ok := retryAfterOf(lastErr); ok && ra > delay {
				if ra > maxRetryAfter {
					ra = maxRetryAfter
				}
				delay = ra
			}
			log.WarnContext(ctx, "llm call attempt failed, retrying after backoff",
				"provider", provider,
				"attempt", attempt,
				"delay_ms", delay.Milliseconds(),
				"status", statusOf(lastErr),
				"prev_error", lastErr,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		if err := ctx.Err(); err != nil {
			log.DebugContext(ctx, "llm retry loop aborted by context", "provider", provider, "err", err)
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !shouldRetry(lastErr) {
			return lastErr
		}
	}
	return lastErr
}
