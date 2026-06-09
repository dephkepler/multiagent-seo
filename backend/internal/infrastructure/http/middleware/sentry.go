package middleware

import (
	"net/http"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"

	"multiagent-seo/pkg/logger"
)

func SentryMiddleware() func(http.Handler) http.Handler {
	handler := sentryhttp.New(sentryhttp.Options{
		Repanic: true,
	})
	return handler.Handle
}

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
