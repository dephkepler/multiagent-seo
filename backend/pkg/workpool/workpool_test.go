package workpool

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestRun_ProcessesAllInBatches(t *testing.T) {
	items := make([]int, 50)
	for i := range items {
		items[i] = i
	}

	var mu sync.Mutex
	var sunk []int
	batches := 0

	processed, err := Run(context.Background(), items,
		func(_ context.Context, n int) (int, bool) { return n * 2, true },
		func(_ context.Context, batch []int) error {
			mu.Lock()
			defer mu.Unlock()
			batches++
			sunk = append(sunk, batch...)
			return nil
		},
		Options{Concurrency: 4, FlushEvery: 10},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if processed != 50 {
		t.Errorf("processed = %d, want 50", processed)
	}
	if len(sunk) != 50 {
		t.Errorf("sunk = %d, want 50", len(sunk))
	}
	if batches < 5 {
		t.Errorf("batches = %d, want >= 5", batches)
	}
}

func TestRun_SkipsWhenNotOK(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6}
	var mu sync.Mutex
	var sunk []int

	processed, err := Run(context.Background(), items,
		func(_ context.Context, n int) (int, bool) { return n, n%2 == 0 },
		func(_ context.Context, batch []int) error {
			mu.Lock()
			defer mu.Unlock()
			sunk = append(sunk, batch...)
			return nil
		},
		Options{Concurrency: 2, FlushEvery: 2},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if processed != 3 {
		t.Errorf("processed = %d, want 3 (only even kept)", processed)
	}
	if len(sunk) != 3 {
		t.Errorf("sunk = %d, want 3", len(sunk))
	}
}

func TestRun_AbortsOnSinkError(t *testing.T) {
	items := make([]int, 100)
	wantErr := errors.New("sink boom")

	_, err := Run(context.Background(), items,
		func(_ context.Context, n int) (int, bool) { return n, true },
		func(_ context.Context, _ []int) error { return wantErr },
		Options{Concurrency: 4, FlushEvery: 5},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want sink boom", err)
	}
}

func TestRun_ReturnsCtxErrOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	items := make([]int, 100)
	_, err := Run(ctx, items,
		func(c context.Context, n int) (int, bool) { return n, c.Err() == nil },
		func(_ context.Context, _ []int) error { return nil },
		Options{Concurrency: 4, FlushEvery: 5},
	)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
