package jobrunner

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// JobRunner dispatches background generation jobs. Each job runs in its own
// goroutine with panic recovery and a fresh, request-decoupled context bounded
// by Timeout; a WaitGroup tracks in-flight jobs so Wait can drain them on
// shutdown (mirrors the legacy extraction-worker drain).
type JobRunner interface {
	// Go runs fn in the background. The context fn receives is derived from
	// parent via WithoutCancel (so request cancellation doesn't abort the job)
	// and capped by the runner's timeout.
	Go(parent context.Context, fn func(context.Context))
	// Wait blocks until all in-flight jobs finish or ctx fires, whichever first.
	Wait(ctx context.Context) error
}

// AsyncRunner is the production JobRunner: real goroutines, per-job timeout,
// panic recovery, drain-on-shutdown.
type AsyncRunner struct {
	timeout time.Duration
	log     *slog.Logger
	wg      sync.WaitGroup
}

func NewAsyncRunner(timeout time.Duration, log *slog.Logger) *AsyncRunner {
	if log == nil {
		log = slog.Default()
	}
	return &AsyncRunner{timeout: timeout, log: log}
}

func (r *AsyncRunner) Go(parent context.Context, fn func(context.Context)) {
	// WithoutCancel: a job outlives the HTTP request that triggered it, but we
	// keep parent's values (e.g. request_id) for log correlation.
	base := context.WithoutCancel(parent)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ctx := base
		var cancel context.CancelFunc
		if r.timeout > 0 {
			ctx, cancel = context.WithTimeout(base, r.timeout)
			defer cancel()
		}
		defer func() {
			if rec := recover(); rec != nil {
				r.log.ErrorContext(ctx, "background job panic",
					"err", rec,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn(ctx)
	}()
}

func (r *AsyncRunner) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SyncRunner runs jobs inline on the calling goroutine. It is the test seam:
// Generate dispatches through JobRunner, so a test using SyncRunner observes
// the full pipeline run before Generate returns.
type SyncRunner struct{}

func NewSyncRunner() SyncRunner { return SyncRunner{} }

func (SyncRunner) Go(parent context.Context, fn func(context.Context)) {
	fn(context.WithoutCancel(parent))
}

func (SyncRunner) Wait(context.Context) error { return nil }
