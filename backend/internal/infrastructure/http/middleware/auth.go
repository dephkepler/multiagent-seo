package middleware

import (
	"context"
	"net/http"
	"strings"

	"multiagent-seo/internal/domain/auth"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

// BearerAuth enforces auth on routes marked with the bearerAuth security scheme.
// oapi-codegen sets BearerAuthScopes in context for those routes before this runs,
// so routes without it (healthz, login) pass through untouched.
func BearerAuth(verifier auth.TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, requiresAuth := r.Context().Value(oapigen.BearerAuthScopes).([]string)
			if !requiresAuth {
				next.ServeHTTP(w, r)
				return
			}

			token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !found || token == "" {
				problem.Write(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			userID, err := verifier.Verify(r.Context(), token)
			if err != nil {
				problem.Write(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			ctx := context.WithValue(r.Context(), logger.ContextKeyUserID, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	uid, ok := ctx.Value(logger.ContextKeyUserID).(string)
	return uid, ok && uid != ""
}
