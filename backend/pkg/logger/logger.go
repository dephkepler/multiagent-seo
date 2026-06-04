package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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

var slogLevel = new(slog.LevelVar)

func Init(level string) error {
	zl, sl, err := ParseLevel(level)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(zl)
	slogLevel.Set(sl)
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	return nil
}

func SetLevel(level string) error {
	zl, sl, err := ParseLevel(level)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(zl)
	slogLevel.Set(sl)
	return nil
}

func CurrentLevel() string {
	switch slogLevel.Level() {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	default:
		return "info"
	}
}

func ParseLevel(level string) (zerolog.Level, slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zerolog.DebugLevel, slog.LevelDebug, nil
	case "", "info":
		return zerolog.InfoLevel, slog.LevelInfo, nil
	case "warn", "warning":
		return zerolog.WarnLevel, slog.LevelWarn, nil
	case "error":
		return zerolog.ErrorLevel, slog.LevelError, nil
	default:
		return zerolog.InfoLevel, slog.LevelInfo, fmt.Errorf("unknown log level %q", level)
	}
}

func New(ctx context.Context, module string) zerolog.Logger {
	return newWithWriter(ctx, os.Stdout, module)
}

func newWithWriter(ctx context.Context, out io.Writer, module string) zerolog.Logger {
	if out == nil {
		out = os.Stdout
	}
	logCtx := zerolog.New(out).With().Timestamp()
	if v, ok := ctx.Value(ContextKeyTraceID).(string); ok && v != "" {
		logCtx = logCtx.Str(string(ContextKeyTraceID), v)
	}
	if v, ok := ctx.Value(ContextKeySpanID).(string); ok && v != "" {
		logCtx = logCtx.Str(string(ContextKeySpanID), v)
	}
	if v, ok := ctx.Value(ContextKeyUserID).(string); ok && v != "" {
		logCtx = logCtx.Str(string(ContextKeyUserID), v)
	}
	return logCtx.Str("module", module).Logger()
}

func NewSlog() *slog.Logger {
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel})
	return slog.New(&contextHandler{Handler: base})
}

type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, rec slog.Record) error {
	for _, key := range []ContextKey{ContextKeyTraceID, ContextKeySpanID, ContextKeyUserID} {
		if v, ok := ctx.Value(key).(string); ok && v != "" {
			rec.AddAttrs(slog.String(string(key), v))
		}
	}
	return h.Handler.Handle(ctx, rec)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

func ContextWith(ctx context.Context, key ContextKey, value string) context.Context {
	return context.WithValue(ctx, key, value)
}
