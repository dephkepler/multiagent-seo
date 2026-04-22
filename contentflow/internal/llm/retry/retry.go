// Package retry provides exponential-backoff retry for LLM HTTP calls,
// shared by all provider subpackages (groq, claude, ...).
//
// It lives in its own subpackage (rather than directly in package llm) to
// avoid an import cycle: provider subpackages need retry helpers, and the
// llm package's factory.go needs to import provider subpackages.
package retry

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Config controls retry behaviour.
type Config struct {
	MaxAttempts int
	Backoffs    []time.Duration
}

// Default returns 3 attempts with backoffs 1s, 2s, 4s between them.
func Default() Config {
	return Config{
		MaxAttempts: 3,
		Backoffs:    []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
	}
}

// HTTPStatusError is implemented by errors that carry an HTTP status code.
// Provider clients return errors satisfying this interface so Do can decide
// whether a failure is retryable.
//
// Status 0 means transport-level failure (no HTTP response received).
type HTTPStatusError interface {
	error
	HTTPStatus() int
}

// shouldRetry returns true for transient failures worth retrying:
//   - transport-level failures (HTTPStatus() == 0)
//   - 429 Too Many Requests
//   - 5xx Server Error
//
// Other 4xx and plain errors (not implementing HTTPStatusError) are terminal.
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

// Do runs fn up to cfg.MaxAttempts times with exponential backoff between
// attempts. It respects ctx.Done() — if the context is cancelled during a
// backoff sleep or before an attempt, it returns ctx.Err() immediately.
// Non-retryable errors short-circuit the loop and are returned as-is.
//
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
			log.Warn("llm retry backoff",
				"provider", provider,
				"attempt", attempt,
				"delay_ms", delay.Milliseconds(),
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
