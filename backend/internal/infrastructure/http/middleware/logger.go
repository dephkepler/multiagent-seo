package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"multiagent-seo/pkg/logger"
)

const awsTraceHeader = "X-Amzn-Trace-Id"

// userIDHolder is a request-scoped mailbox: RequestLogger runs before auth, so it
// seeds an empty holder into the context that BearerAuth (downstream) fills once
// it knows the user. The deferred access-log line then reports user_id too.
type userIDHolder struct{ id string }

type ctxKey int

const userIDHolderKey ctxKey = iota

// RequestLogger reuses the upstream trace ID (e.g. from the load balancer) when present
// so logs stitch into the same distributed trace, generating one only as a fallback.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		traceID := r.Header.Get(awsTraceHeader)
		if traceID == "" {
			traceID = generateID()
		}

		ctx := context.WithValue(r.Context(), logger.ContextKeyTraceID, traceID)
		ctx = context.WithValue(ctx, logger.ContextKeySpanID, chiMiddleware.GetReqID(r.Context()))
		holder := &userIDHolder{}
		ctx = context.WithValue(ctx, userIDHolderKey, holder)
		r = r.WithContext(ctx)

		// Defer so the access-log line is always emitted, even when the handler panics.
		defer func() {
			rec := recover()

			// BearerAuth (downstream) filled the holder by now; fold user_id in.
			logCtx := ctx
			if holder.id != "" {
				logCtx = context.WithValue(ctx, logger.ContextKeyUserID, holder.id)
			}
			log := logger.New(logCtx, "http")
			status := ww.Status()
			var event *zerolog.Event
			switch {
			case rec != nil || status >= 500:
				event = log.Error()
			case status >= 400:
				event = log.Warn()
			default:
				event = log.Info()
			}
			if rec != nil {
				// Stack is dumped by chi Recoverer/Sentry, so log only the panic value here.
				event = event.Interface("panic", rec)
			}
			event.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", status).
				Dur("duration", time.Since(start)).
				Msg("request")

			if rec != nil {
				panic(rec) // re-panic so SentryMiddleware (Repanic) and chi Recoverer still handle it
			}
		}()

		next.ServeHTTP(ww, r)
	})
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
