package app

import (
	"context"
	"time"
)

func schedule(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				fn(ctx)
			}
		}
	}()
}
