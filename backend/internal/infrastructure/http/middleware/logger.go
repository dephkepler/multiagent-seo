package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"multiagent-seo/pkg/logger"
)

const awsTraceHeader = "X-Amzn-Trace-Id"

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
		r = r.WithContext(ctx)

		next.ServeHTTP(ww, r)

		log := logger.New(ctx, "http")
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", ww.Status()).
			Dur("duration", time.Since(start)).
			Msg("request")
	})
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
