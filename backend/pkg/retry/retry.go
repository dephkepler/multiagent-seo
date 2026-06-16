package retry

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

type Config struct {
	MaxAttempts int
	Backoffs    []time.Duration
	Jitter      bool
	// MaxRetryAfter caps how long a server's Retry-After can stall us; 0 uses DefaultMaxRetryAfter.
	MaxRetryAfter time.Duration
}

const DefaultMaxRetryAfter = 30 * time.Second

func Default() Config {
	return Config{
		MaxAttempts: 5,
		Backoffs:    []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second},
		Jitter:      true,
	}
}

type HTTPStatusError interface {
	error
	HTTPStatus() int
}

type RetryAfterError interface {
	RetryAfter() (time.Duration, bool)
}

// ParseRetryAfter reads a Retry-After header in either accepted form:
// delay-seconds or an HTTP-date.
func ParseRetryAfter(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		if secs > 0 {
			return time.Duration(secs * float64(time.Second))
		}
		return 0
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
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
	var he HTTPStatusError
	if err == nil || !errors.As(err, &he) {
		return false
	}
	status := he.HTTPStatus()
	if status == 0 || status == 429 {
		return true
	}
	return status >= 500 && status < 600
}

func Do(ctx context.Context, cfg Config, log *slog.Logger, label string, fn func() error) error {
	if log == nil {
		log = slog.Default()
	}
	maxRetryAfter := cfg.MaxRetryAfter
	if maxRetryAfter <= 0 {
		maxRetryAfter = DefaultMaxRetryAfter
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if attempt > 1 {
			delay := backoffFor(cfg, attempt)
			if cfg.Jitter {
				delay = withJitter(delay)
			}
			if ra, ok := retryAfterOf(lastErr); ok && ra > delay {
				if ra > maxRetryAfter {
					ra = maxRetryAfter
				}
				delay = ra
			}
			log.WarnContext(ctx, "retryable call failed, backing off",
				"label", label,
				"attempt", attempt,
				"delay_ms", delay.Milliseconds(),
				"status", statusOf(lastErr),
				"err", lastErr,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		if err := ctx.Err(); err != nil {
			log.DebugContext(ctx, "retry loop aborted by context", "label", label, "err", err)
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

func backoffFor(cfg Config, attempt int) time.Duration {
	if len(cfg.Backoffs) == 0 {
		return 0
	}
	idx := attempt - 2
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cfg.Backoffs) {
		idx = len(cfg.Backoffs) - 1
	}
	return cfg.Backoffs[idx]
}

// withJitter spreads retries across [delay/2, delay] so many clients retrying at
// once don't stampede the same server on the same schedule.
func withJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}
