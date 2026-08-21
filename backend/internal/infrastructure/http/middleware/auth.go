package middleware

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"multiagent-seo/internal/domain/auth"
	"multiagent-seo/internal/domain/user"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

type principalKeyType struct{}

var principalKey principalKeyType

// Principal is who is making the request. The token carries only an id, so the
// role is read from the user row on every request: it costs one indexed SELECT
// at this scale, it means a role change takes effect immediately rather than
// when the token expires, and — the reason that matters — it closes the gap
// where an API key (mas_…) was indistinguishable from a browser session,
// because both now resolve to the same user and inherit that user's role.
type Principal struct {
	UserID string
	Role   user.Role
	// AdvocateID is the roster row this caller speaks for; empty for admins.
	AdvocateID string
}

// UserLookup is the middleware's view of the user store — one method, declared
// where it is consumed.
type UserLookup interface {
	FindByID(ctx context.Context, id string) (user.User, error)
}

func BearerAuth(verifier auth.TokenVerifier, users UserLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes, requiresAuth := r.Context().Value(oapigen.BearerAuthScopes).([]string)
			if !requiresAuth {
				next.ServeHTTP(w, r)
				return
			}

			token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !found || token == "" {
				log := logger.New(r.Context(), "middleware.auth")
				log.Warn().Str("reason", "missing_token").Msg("unauthorized")
				problem.Write(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			userID, err := verifier.Verify(r.Context(), token)
			if err != nil {
				log := logger.New(r.Context(), "middleware.auth")
				log.Warn().Str("reason", "verify_failed").Err(err).Msg("unauthorized")
				problem.Write(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			caller, err := users.FindByID(r.Context(), userID)
			if err != nil {
				// A token that verifies but names nobody is not an authorization
				// question — the account is gone, so the session is over.
				log := logger.New(r.Context(), "middleware.auth")
				log.Warn().Str("reason", "user_not_found").Err(err).Msg("unauthorized")
				problem.Write(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			if !allowedByScopes(caller.Role, scopes) {
				log := logger.New(r.Context(), "middleware.auth")
				log.Warn().Str("role", string(caller.Role)).Strs("scopes", scopes).Msg("forbidden")
				problem.Write(w, http.StatusForbidden, "forbidden")
				return
			}

			if h, ok := r.Context().Value(userIDHolderKey).(*userIDHolder); ok {
				h.id = userID
			}
			ctx := context.WithValue(r.Context(), logger.ContextKeyUserID, userID)
			ctx = context.WithValue(ctx, principalKey, Principal{
				UserID:     userID,
				Role:       caller.Role,
				AdvocateID: caller.AdvocateID,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// allowedByScopes fails closed: an operation that names no role is admin-only.
// The alternative — an empty list meaning "anyone authenticated" — is how a
// forgotten or newly added endpoint silently becomes readable by an advocate,
// and that is precisely the leak this gate exists to prevent.
func allowedByScopes(role user.Role, scopes []string) bool {
	if len(scopes) == 0 {
		return role == user.RoleAdmin
	}
	return slices.Contains(scopes, string(role))
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	uid, ok := ctx.Value(logger.ContextKeyUserID).(string)
	return uid, ok && uid != ""
}

// PrincipalFromContext is what an advocate-scoped handler filters on. The second
// return is false for unauthenticated routes, and a handler that needs a scope
// must refuse the request rather than fall back to "no filter".
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok && p.UserID != ""
}
