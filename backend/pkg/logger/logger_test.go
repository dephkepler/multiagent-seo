package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func decodeFirst(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	dec := json.NewDecoder(buf)
	var got map[string]any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("decode log line: %v\nraw: %s", err, buf.String())
	}
	return got
}

func TestNew_WritesModule(t *testing.T) {
	var buf bytes.Buffer
	l := newWithWriter(context.Background(), &buf, "api.health.get")
	l.Info().Msg("hello")

	got := decodeFirst(t, &buf)
	if got["module"] != "api.health.get" {
		t.Errorf("module = %v, want %q", got["module"], "api.health.get")
	}
	if got["message"] != "hello" {
		t.Errorf("message = %v, want %q", got["message"], "hello")
	}
}

func TestNew_PullsContextKeys(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWith(ctx, ContextKeyTraceID, "trace-abc")
	ctx = ContextWith(ctx, ContextKeySpanID, "span-xyz")
	ctx = ContextWith(ctx, ContextKeyUserID, "user-42")

	var buf bytes.Buffer
	l := newWithWriter(ctx, &buf, "api.users.create")
	l.Info().Msg("event")

	got := decodeFirst(t, &buf)
	if got["trace_id"] != "trace-abc" {
		t.Errorf("trace_id = %v, want %q", got["trace_id"], "trace-abc")
	}
	if got["span_id"] != "span-xyz" {
		t.Errorf("span_id = %v, want %q", got["span_id"], "span-xyz")
	}
	if got["user_id"] != "user-42" {
		t.Errorf("user_id = %v, want %q", got["user_id"], "user-42")
	}
}

func TestNew_SkipsEmptyContextValues(t *testing.T) {
	ctx := ContextWith(context.Background(), ContextKeyTraceID, "")

	var buf bytes.Buffer
	l := newWithWriter(ctx, &buf, "x")
	l.Info().Msg("ev")

	got := decodeFirst(t, &buf)
	if _, present := got["trace_id"]; present {
		t.Errorf("empty trace_id should not be emitted, got %v", got["trace_id"])
	}
}

func TestContextHandler_InjectsIDsAndSurvivesWith(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(&contextHandler{Handler: slog.NewJSONHandler(&buf, nil)})

	ctx := ContextWith(context.Background(), ContextKeyTraceID, "trace-1")
	ctx = ContextWith(ctx, ContextKeyUserID, "user-9")

	// .With must keep the decoration so infra loggers (built once, then .With'd
	// per job) still correlate.
	l.With("article_id", 7).InfoContext(ctx, "hello")

	got := decodeFirst(t, &buf)
	if got["trace_id"] != "trace-1" {
		t.Errorf("trace_id = %v, want trace-1", got["trace_id"])
	}
	if got["user_id"] != "user-9" {
		t.Errorf("user_id = %v, want user-9", got["user_id"])
	}
	if got["article_id"] != float64(7) {
		t.Errorf("article_id = %v, want 7", got["article_id"])
	}
}

func TestContextHandler_NoIDsOnNonContextCall(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(&contextHandler{Handler: slog.NewJSONHandler(&buf, nil)})

	// Non-context Info passes a background context — no IDs to inject.
	l.Info("plain")

	got := decodeFirst(t, &buf)
	if _, ok := got["trace_id"]; ok {
		t.Errorf("trace_id should be absent, got %v", got["trace_id"])
	}
}

func TestSetLevel_RoundTrip(t *testing.T) {
	t.Cleanup(func() { _ = SetLevel("info") })
	for _, want := range []string{"debug", "warn", "error", "info"} {
		if err := SetLevel(want); err != nil {
			t.Fatalf("SetLevel(%q): %v", want, err)
		}
		if got := CurrentLevel(); got != want {
			t.Errorf("CurrentLevel() = %q after SetLevel(%q)", got, want)
		}
	}
	if err := SetLevel("nonsense"); err == nil {
		t.Error("SetLevel(\"nonsense\") = nil, want error")
	}
}
