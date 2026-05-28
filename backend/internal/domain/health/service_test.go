package health_test

import (
	"context"
	"errors"
	"testing"

	"multiagent-seo/internal/domain/health"
)

type stubRepo struct{ err error }

func (s stubRepo) Ping(context.Context) error { return s.err }

func TestCheck(t *testing.T) {
	ctx := context.Background()

	t.Run("nil repo -> unknown/degraded", func(t *testing.T) {
		h := health.NewService(nil).Check(ctx)
		if h.DB() != health.DBStatusUnknown || h.IsHealthy() {
			t.Errorf("got db=%s healthy=%v", h.DB(), h.IsHealthy())
		}
	})

	t.Run("ping ok -> ok/healthy", func(t *testing.T) {
		h := health.NewService(stubRepo{nil}).Check(ctx)
		if h.DB() != health.DBStatusOK || !h.IsHealthy() {
			t.Errorf("got db=%s healthy=%v", h.DB(), h.IsHealthy())
		}
	})

	t.Run("ping err -> down/degraded", func(t *testing.T) {
		h := health.NewService(stubRepo{errors.New("conn refused")}).Check(ctx)
		if h.DB() != health.DBStatusDown || h.IsHealthy() {
			t.Errorf("got db=%s healthy=%v", h.DB(), h.IsHealthy())
		}
	})
}
