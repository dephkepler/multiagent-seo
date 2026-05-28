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
		MaxAttempts: 3,
		Backoffs:    []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
	}
}

// HTTPStatus() == 0 signals a transport-level failure (no HTTP response received).
type HTTPStatusError interface {
	error
	HTTPStatus() int
}

// statusOf extracts the HTTP status from a HTTPStatusError; 0 if absent/transport-level.
func statusOf(err error) int {
	var he HTTPStatusError
	if errors.As(err, &he) {
		return he.HTTPStatus()
	}
	return 0
}

// Retryable: transport failures (status 0), 429, and 5xx. Everything else is terminal.
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

// provider is used only for log fields.
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
			// Retries on 429/5xx/transport are expected; log at Info, not Warn.
			log.InfoContext(ctx, "llm retry backoff",
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
