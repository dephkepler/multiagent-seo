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

// slogLevel is the live level for slog loggers. SetLevel updates it atomically,
// so verbosity can change at runtime without rebuilding any logger.
var slogLevel = new(slog.LevelVar)

// Init applies the initial level to both zerolog (global) and slog, and installs
// a JSON zerolog logger on stdout. Call once at startup after config is loaded.
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

// SetLevel changes the active level for both loggers at runtime.
func SetLevel(level string) error {
	zl, sl, err := ParseLevel(level)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(zl)
	slogLevel.Set(sl)
	return nil
}

// CurrentLevel reports the active level as a lowercase name.
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

// ParseLevel maps a name (debug|info|warn|error) to zerolog and slog levels.
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

// New returns a request-scoped zerolog logger (JSON on stdout) tagged with the
// module and any trace/span/user IDs present in ctx. Level follows the global level.
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

// NewSlog builds the application slog logger: JSON on stdout at the shared level
// (SetLevel affects it at runtime) and trace-aware. Because the handler reads the
// trace/span/user IDs from the context on every record, callers must use the
// *Context log methods (InfoContext, ErrorContext, ...) for correlation — the
// non-context methods pass a background context and so log without IDs.
func NewSlog() *slog.Logger {
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel})
	return slog.New(&contextHandler{Handler: base})
}

// contextHandler decorates each record with the trace/span/user IDs present in
// the call's context, so infrastructure adapters (which hold a logger built once
// at boot) still emit correlated logs during async jobs.
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

// WithAttrs/WithGroup must re-wrap so the decoration survives logger.With(...);
// the embedded handler's methods would otherwise return a bare base handler.
func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

func ContextWith(ctx context.Context, key ContextKey, value string) context.Context {
	return context.WithValue(ctx, key, value)
}
