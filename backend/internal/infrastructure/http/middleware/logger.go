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

type userIDHolder struct{ id string }

type ctxKey int

const userIDHolderKey ctxKey = iota

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

		defer func() {
			rec := recover()

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
				event = event.Interface("panic", rec)
			}
			event.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", status).
				Dur("duration", time.Since(start)).
				Msg("request")

			if rec != nil {
				panic(rec)
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
