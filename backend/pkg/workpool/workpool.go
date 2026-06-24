package workpool

import (
	"context"
	"sync"
	"time"
)

type Options struct {
	Concurrency  int           // max in-flight workers; <= 0 means 1
	FlushEvery   int           // sink batch size; <= 0 flushes only once at the end
	FlushTimeout time.Duration // per-flush timeout; <= 0 means none
}

func Run[T any, R any](
	ctx context.Context,
	items []T,
	work func(ctx context.Context, item T) (R, bool),
	sink func(ctx context.Context, batch []R) error,
	opts Options,
) (int, error) {
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultsCh := make(chan R)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	go func() {
		for i := range items {
			select {
			case <-workCtx.Done():
				wg.Wait()
				close(resultsCh)
				return
			case sem <- struct{}{}:
			}
			wg.Add(1)
			go func(item T) {
				defer wg.Done()
				defer func() { <-sem }()
				if r, ok := work(workCtx, item); ok {
					select {
					case resultsCh <- r:
					case <-workCtx.Done():
					}
				}
			}(items[i])
		}
		wg.Wait()
		close(resultsCh)
	}()

	batch := make([]R, 0, max(opts.FlushEvery, 0))
	processed := 0
	var sinkErr error

	flush := func() {
		if len(batch) == 0 {
			return
		}
		fctx, fcancel := flushContext(ctx, opts.FlushTimeout)
		defer fcancel()
		if err := sink(fctx, batch); err != nil {
			sinkErr = err
			cancel()
			return
		}
		batch = batch[:0]
	}

	for r := range resultsCh {
		if sinkErr != nil {
			continue // keep draining so producers unblock and exit
		}
		batch = append(batch, r)
		processed++
		if opts.FlushEvery > 0 && len(batch) >= opts.FlushEvery {
			flush()
		}
	}
	if sinkErr != nil {
		return processed, sinkErr
	}
	flush()
	if sinkErr != nil {
		return processed, sinkErr
	}
	if err := ctx.Err(); err != nil {
		return processed, err
	}
	return processed, nil
}

func flushContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(ctx)
	if timeout > 0 {
		return context.WithTimeout(base, timeout)
	}
	return base, func() {}
}
