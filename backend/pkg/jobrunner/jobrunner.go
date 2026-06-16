package jobrunner

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

type JobRunner interface {
	Go(parent context.Context, fn func(context.Context))
	Wait(ctx context.Context) error
}

type AsyncRunner struct {
	timeout time.Duration
	log     *slog.Logger
	wg      sync.WaitGroup
	sem     chan struct{}
}

func NewAsyncRunner(timeout time.Duration, maxConcurrent int, log *slog.Logger) *AsyncRunner {
	if log == nil {
		log = slog.Default()
	}
	var sem chan struct{}
	if maxConcurrent > 0 {
		sem = make(chan struct{}, maxConcurrent)
	}
	return &AsyncRunner{timeout: timeout, log: log, sem: sem}
}

func (r *AsyncRunner) Go(parent context.Context, fn func(context.Context)) {
	base := context.WithoutCancel(parent)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if r.sem != nil {
			r.sem <- struct{}{}
			defer func() { <-r.sem }()
		}
		ctx := base
		var cancel context.CancelFunc
		if r.timeout > 0 {
			ctx, cancel = context.WithTimeout(base, r.timeout)
			defer cancel()
		}
		defer func() {
			if rec := recover(); rec != nil {
				r.log.ErrorContext(ctx, "background job panic",
					"panic", rec,
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

type SyncRunner struct{}

func NewSyncRunner() SyncRunner { return SyncRunner{} }

func (SyncRunner) Go(parent context.Context, fn func(context.Context)) {
	fn(context.WithoutCancel(parent))
}

func (SyncRunner) Wait(context.Context) error { return nil }
