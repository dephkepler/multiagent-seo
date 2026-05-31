package middleware

import (
	"net/http"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"

	"multiagent-seo/pkg/logger"
)

// SentryMiddleware captures panics and errors. Safe to always wire in: Sentry
// no-ops when not initialized. Repanic lets the chi Recoverer still handle the panic.
func SentryMiddleware() func(http.Handler) http.Handler {
	handler := sentryhttp.New(sentryhttp.Options{
		Repanic: true,
	})
	return handler.Handle
}

// SentryScopeEnhancer enriches the Sentry scope with trace_id and span_id from context.
// Must be registered after RequestLogger so trace context is already set on the request context.
func SentryScopeEnhancer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.New(ctx, "middleware.sentry")

		hub := sentry.GetHubFromContext(ctx)
		if hub == nil {
			log.Debug().Msg("Sentry hub not available, skipping scope enhancement")
			next.ServeHTTP(w, r)
			return
		}

		hub.Scope().SetTag("trace_id", stringFromCtx(ctx, logger.ContextKeyTraceID))
		hub.Scope().SetTag("span_id", stringFromCtx(ctx, logger.ContextKeySpanID))
		log.Debug().Msg("Sentry scope enhanced with trace context")

		next.ServeHTTP(w, r)
	})
}
