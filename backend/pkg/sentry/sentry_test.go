package sentry

import (
	"testing"

	"multiagent-seo/pkg/config"
)

func TestInitialize_DisabledSkips(t *testing.T) {
	cfg := config.SentryConfig{Enabled: false}
	s := New(cfg)

	if err := s.Initialize(); err != nil {
		t.Fatalf("expected nil err on disabled init, got %v", err)
	}
}
