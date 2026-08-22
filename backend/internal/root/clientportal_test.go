package root

import (
	"io"
	"log/slog"
	"testing"
	_ "time/tzdata"

	appleads "multiagent-seo/internal/application/webleads"
	"multiagent-seo/pkg/config"
)

// The prod runtime image is bare alpine with no zone database, so this used to
// fail there and take the whole client portal down with a single warning — every
// booking request answered 503. The blank import of time/tzdata in cmd/server is
// what fixes it; this pins that the name we ship in the default config is one
// the binary can actually resolve.
func TestScheduleTimezoneResolves(t *testing.T) {
	cfg := config.Config{}
	cfg.Schedule.Timezone = "Europe/Kyiv"

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	// A nil lead pipeline short-circuits before the timezone is read, so this
	// test needs a non-nil one to reach it. It is never called.
	if _, err := buildClientPortal(cfg, quiet, nil, &appleads.Service{}); err != nil {
		t.Fatalf("the shipped default timezone does not resolve: %v", err)
	}
}

func TestUnknownScheduleTimezoneIsFatal(t *testing.T) {
	cfg := config.Config{}
	cfg.Schedule.Timezone = "Europe/Nowhere"

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := buildClientPortal(cfg, quiet, nil, &appleads.Service{}); err == nil {
		t.Fatal("err = nil, want a refusal — a typo here must not disable the portal quietly")
	}
}
