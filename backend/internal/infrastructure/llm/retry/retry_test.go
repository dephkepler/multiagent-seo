package retry

import (
	"context"
	"testing"
	"time"
)

type fakeErr struct {
	status int
	after  time.Duration
}

func (e fakeErr) Error() string   { return "fake" }
func (e fakeErr) HTTPStatus() int { return e.status }
func (e fakeErr) RetryAfter() (time.Duration, bool) {
	return e.after, e.after > 0
}

func TestDo_RetriesThenSucceeds(t *testing.T) {
	cfg := Config{MaxAttempts: 3, Backoffs: []time.Duration{time.Millisecond}}
	calls := 0
	err := Do(context.Background(), cfg, nil, "test", func() error {
		calls++
		if calls < 2 {
			return fakeErr{status: 429, after: time.Millisecond}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestDo_DoesNotRetryTerminal(t *testing.T) {
	cfg := Config{MaxAttempts: 3, Backoffs: []time.Duration{time.Millisecond}}
	calls := 0
	err := Do(context.Background(), cfg, nil, "test", func() error {
		calls++
		return fakeErr{status: 400} // 4xx (not 429) is terminal
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 4xx)", calls)
	}
}

func TestDo_HonorsRetryAfterOverBackoff(t *testing.T) {
	// Backoff is tiny; Retry-After is larger → Do should wait at least the
	// Retry-After before the second attempt.
	cfg := Config{MaxAttempts: 2, Backoffs: []time.Duration{time.Microsecond}}
	calls := 0
	start := time.Now()
	_ = Do(context.Background(), cfg, nil, "test", func() error {
		calls++
		if calls == 1 {
			return fakeErr{status: 429, after: 30 * time.Millisecond}
		}
		return nil
	})
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Errorf("waited %v, expected ~Retry-After (30ms)", elapsed)
	}
}
