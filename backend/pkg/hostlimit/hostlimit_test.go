package hostlimit

import (
	"context"
	"errors"
	"testing"
)

func TestUnlimitedReturnsImmediately(t *testing.T) {
	l := New(0, 0)
	for i := 0; i < 100; i++ {
		if err := l.Wait(context.Background(), "example.com"); err != nil {
			t.Fatalf("Wait: %v", err)
		}
	}
}

func TestPerHostIndependent(t *testing.T) {
	l := New(1, 1) // one per second, burst 1
	if err := l.Wait(context.Background(), "a.com"); err != nil {
		t.Fatalf("a.com first Wait: %v", err)
	}
	// Different host has its own bucket, so its first token is available now.
	if err := l.Wait(context.Background(), "b.com"); err != nil {
		t.Fatalf("b.com first Wait: %v", err)
	}
}

func TestWaitRespectsContext(t *testing.T) {
	l := New(0.5, 1) // one per 2s, burst 1
	if err := l.Wait(context.Background(), "slow.com"); err != nil {
		t.Fatalf("first Wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Second token isn't available for ~2s; a canceled ctx must return promptly.
	if err := l.Wait(ctx, "slow.com"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
