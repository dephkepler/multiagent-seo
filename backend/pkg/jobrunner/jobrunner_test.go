package jobrunner

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAsyncRunner_Go_CapsConcurrencyAtMaxConcurrent(t *testing.T) {
	const max = 3
	const n = 10
	r := NewAsyncRunner(0, max, nil)

	var cur, peak, ran atomic.Int32
	started := make(chan struct{}, n)
	release := make(chan struct{})

	fn := func(context.Context) {
		ran.Add(1)
		c := cur.Add(1)
		for {
			p := peak.Load()
			if c <= p || peak.CompareAndSwap(p, c) {
				break
			}
		}
		started <- struct{}{}
		<-release
		cur.Add(-1)
	}

	for i := 0; i < n; i++ {
		r.Go(context.Background(), fn)
	}

	for i := 0; i < max; i++ {
		<-started
	}
	if got := peak.Load(); got > max {
		close(release)
		t.Fatalf("peak concurrency %d exceeded max %d", got, max)
	}
	close(release)

	if err := r.Wait(context.Background()); err != nil {
		t.Fatalf("Wait returned %v, want nil", err)
	}
	if got := peak.Load(); got != max {
		t.Errorf("peak = %d, want %d", got, max)
	}
	if got := cur.Load(); got != 0 {
		t.Errorf("cur = %d, want 0", got)
	}
	if got := ran.Load(); got != n {
		t.Errorf("ran = %d, want %d", got, n)
	}
}

func TestAsyncRunner_Go_UnlimitedWhenMaxConcurrentNonPositive(t *testing.T) {
	for _, maxConcurrent := range []int{0, -1} {
		t.Run("", func(t *testing.T) {
			const n = 8
			r := NewAsyncRunner(0, maxConcurrent, nil)

			var cur, peak atomic.Int32
			started := make(chan struct{}, n)
			release := make(chan struct{})

			fn := func(context.Context) {
				c := cur.Add(1)
				for {
					p := peak.Load()
					if c <= p || peak.CompareAndSwap(p, c) {
						break
					}
				}
				started <- struct{}{}
				<-release
				cur.Add(-1)
			}

			for i := 0; i < n; i++ {
				r.Go(context.Background(), fn)
			}

			for i := 0; i < n; i++ {
				<-started
			}
			if got := peak.Load(); got != n {
				close(release)
				t.Fatalf("peak = %d, want %d (jobs were serialized)", got, n)
			}
			close(release)

			if err := r.Wait(context.Background()); err != nil {
				t.Fatalf("Wait returned %v, want nil", err)
			}
		})
	}
}

func TestAsyncRunner_Wait_ReturnsNilAfterAllDrain(t *testing.T) {
	const n = 20
	r := NewAsyncRunner(0, 2, nil)

	var ran atomic.Int32
	for i := 0; i < n; i++ {
		r.Go(context.Background(), func(context.Context) { ran.Add(1) })
	}

	if err := r.Wait(context.Background()); err != nil {
		t.Fatalf("Wait returned %v, want nil", err)
	}
	if got := ran.Load(); got != n {
		t.Errorf("ran = %d, want %d", got, n)
	}
}

func TestAsyncRunner_Wait_ReturnsCtxErrWhenJobsOutlastDeadline(t *testing.T) {
	r := NewAsyncRunner(0, 1, nil)

	release := make(chan struct{})
	r.Go(context.Background(), func(context.Context) { <-release })

	waitCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.Wait(waitCtx)
	if err != context.Canceled {
		close(release)
		t.Fatalf("Wait returned %v, want context.Canceled", err)
	}

	close(release)
	if err := r.Wait(context.Background()); err != nil {
		t.Errorf("drain Wait returned %v, want nil", err)
	}
}

func TestAsyncRunner_Go_PanicRecoveredAndReleasesSlotAndWaitGroup(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := NewAsyncRunner(0, 1, log)

	var ran atomic.Bool
	r.Go(context.Background(), func(context.Context) { panic("boom") })
	r.Go(context.Background(), func(context.Context) { ran.Store(true) })

	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.Wait(waitCtx); err != nil {
		t.Fatalf("Wait returned %v, want nil (panic likely leaked slot/wg)", err)
	}
	if !ran.Load() {
		t.Error("second job did not run; semaphore slot was not released after panic")
	}
	if !strings.Contains(buf.String(), "background job panic") {
		t.Errorf("expected panic log record, got: %q", buf.String())
	}
}

func TestAsyncRunner_Go_TimeoutContextAppliedToFn(t *testing.T) {
	r := NewAsyncRunner(20*time.Millisecond, 1, nil)

	type result struct {
		hasDeadline bool
		err         error
	}
	res := make(chan result, 1)

	r.Go(context.Background(), func(ctx context.Context) {
		_, ok := ctx.Deadline()
		<-ctx.Done()
		res <- result{hasDeadline: ok, err: ctx.Err()}
	})

	if err := r.Wait(context.Background()); err != nil {
		t.Fatalf("Wait returned %v, want nil", err)
	}
	got := <-res
	if !got.hasDeadline {
		t.Error("fn ctx had no deadline; per-job timeout not applied")
	}
	if got.err != context.DeadlineExceeded {
		t.Errorf("fn ctx.Err() = %v, want context.DeadlineExceeded", got.err)
	}
}

func TestAsyncRunner_Go_UsesContextWithoutCancelFromParent(t *testing.T) {
	r := NewAsyncRunner(0, 1, nil)

	parent, cancelParent := context.WithCancel(context.Background())
	proceed := make(chan struct{})
	res := make(chan error, 1)

	r.Go(parent, func(ctx context.Context) {
		<-proceed
		res <- ctx.Err()
	})

	cancelParent()
	close(proceed)

	if err := r.Wait(context.Background()); err != nil {
		t.Fatalf("Wait returned %v, want nil", err)
	}
	if err := <-res; err != nil {
		t.Errorf("fn ctx.Err() = %v, want nil (parent cancel must be detached)", err)
	}
}

func TestAsyncRunner_NewAsyncRunner_NilLoggerDefaults(t *testing.T) {
	r := NewAsyncRunner(0, 1, nil)

	r.Go(context.Background(), func(context.Context) { panic("boom") })

	if err := r.Wait(context.Background()); err != nil {
		t.Fatalf("Wait returned %v, want nil (nil logger should default, not nil-deref)", err)
	}
}

func TestSyncRunner_GoRunsInlineAndDetachesParentCancel(t *testing.T) {
	r := NewSyncRunner()

	parent, cancel := context.WithCancel(context.Background())
	cancel()

	var ran bool
	var seenErr error
	r.Go(parent, func(ctx context.Context) {
		ran = true
		seenErr = ctx.Err()
	})

	if !ran {
		t.Fatal("SyncRunner.Go did not run fn inline")
	}
	if seenErr != nil {
		t.Errorf("fn ctx.Err() = %v, want nil (parent cancel must be detached)", seenErr)
	}

	if err := r.Wait(context.Background()); err != nil {
		t.Errorf("Wait returned %v, want nil", err)
	}
}
