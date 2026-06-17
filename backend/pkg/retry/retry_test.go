package retry

import (
	"context"
	"net/http"
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
		return fakeErr{status: 400}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 4xx)", calls)
	}
}

func TestDo_HonorsRetryAfterOverBackoff(t *testing.T) {
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

func TestDo_CapsRetryAfter(t *testing.T) {
	cfg := Config{MaxAttempts: 2, Backoffs: []time.Duration{time.Microsecond}, MaxRetryAfter: 10 * time.Millisecond}
	start := time.Now()
	_ = Do(context.Background(), cfg, nil, "test", func() error {
		return fakeErr{status: 429, after: time.Hour}
	})
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("waited %v, expected the cap (10ms) to apply, not the hour Retry-After", elapsed)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := ParseRetryAfter(http.Header{"Retry-After": {"2"}}); d != 2*time.Second {
		t.Errorf("seconds form = %v, want 2s", d)
	}
	if d := ParseRetryAfter(http.Header{}); d != 0 {
		t.Errorf("missing header = %v, want 0", d)
	}
	future := time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)
	if d := ParseRetryAfter(http.Header{"Retry-After": {future}}); d <= 0 || d > 6*time.Second {
		t.Errorf("http-date form = %v, want ~5s", d)
	}
}
