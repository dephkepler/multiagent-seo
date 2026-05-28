package logger

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ContextKey string

const (
	ContextKeyTraceID ContextKey = "trace_id"
	ContextKeySpanID  ContextKey = "span_id"
	ContextKeyUserID  ContextKey = "user_id"
)

const devEnvVar = "CF_LOGGER_DEV"

func Init() {
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}

func New(ctx context.Context, module string) zerolog.Logger {
	return newWithWriter(ctx, os.Stdout, module)
}

func ContextWith(ctx context.Context, key ContextKey, value string) context.Context {
	return context.WithValue(ctx, key, value)
}

func newWithWriter(ctx context.Context, out io.Writer, module string) zerolog.Logger {
	if out == nil {
		out = os.Stdout
	}

	logCtx := log.With()
	// TODO: when OpenTelemetry is wired in, prefer trace.SpanContextFromContext(ctx)
	// over the manual ContextKey lookups below.
	if v, ok := ctx.Value(ContextKeyTraceID).(string); ok && v != "" {
		logCtx = logCtx.Str(string(ContextKeyTraceID), v)
	}
	if v, ok := ctx.Value(ContextKeySpanID).(string); ok && v != "" {
		logCtx = logCtx.Str(string(ContextKeySpanID), v)
	}
	if v, ok := ctx.Value(ContextKeyUserID).(string); ok && v != "" {
		logCtx = logCtx.Str(string(ContextKeyUserID), v)
	}

	l := logCtx.Str("module", module).Logger()

	if isDevMode() {
		return l.Level(zerolog.DebugLevel).Output(zerolog.ConsoleWriter{Out: out})
	}
	return l.Level(zerolog.InfoLevel).Output(out)
}

func isDevMode() bool {
	v, ok := os.LookupEnv(devEnvVar)
	return ok && strings.TrimSpace(v) == "true"
}
