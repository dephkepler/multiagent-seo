package logger

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestIsDevMode(t *testing.T) {
	t.Setenv(devEnvVar, "")
	if isDevMode() {
		t.Errorf("isDevMode() = true, want false when env empty")
	}
	t.Setenv(devEnvVar, "true")
	if !isDevMode() {
		t.Errorf("isDevMode() = false, want true")
	}
	t.Setenv(devEnvVar, "1")
	if isDevMode() {
		t.Errorf("isDevMode() should accept only exact 'true', not %q", "1")
	}
}
