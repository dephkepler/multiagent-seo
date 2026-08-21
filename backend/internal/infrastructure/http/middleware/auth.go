package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"multiagent-seo/internal/domain/auth"
	"multiagent-seo/internal/domain/user"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/tgauth"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

type principalKeyType struct{}

var principalKey principalKeyType

// Authorization schemes this API accepts. Both arrive in the same header, and
// which one it is decides how the caller is identified.
const (
	schemeBearer = "Bearer"
	// schemeTelegram is what Telegram's own SDKs converged on for handing raw
	// init data to a backend. It is not a bearer token: nothing here was ever
	// issued to us, it is a signed snapshot of one Mini App launch.
	schemeTelegram = "tma"
)

// Principal is who is making the request. The token carries only an id, so the
// role is read from the user row on every request: it costs one indexed SELECT
// at this scale, it means a role change takes effect immediately rather than
// when the token expires, and — the reason that matters — it closes the gap
// where an API key (mas_…) was indistinguishable from a browser session,
// because both now resolve to the same user and inherit that user's role.
//
// A Telegram-authenticated caller has no users row at all, so UserID is empty
// there and the role comes from which table the chat id matched.
type Principal struct {
	UserID string
	Role   user.Role
	// AdvocateID is the roster row this caller speaks for; empty for admins.
	AdvocateID string
	// ClientID is the clients row a Telegram-authenticated client speaks for.
	// Empty for everyone else, a guest included — which is why a guest is its
	// own role rather than a client with nothing in here.
	ClientID string
	// TelegramID and TelegramName describe the launch this request came from,
	// and are zero for a password login. The intake operation needs both: it
	// creates the client row and binds it to the chat that reminders go to.
	TelegramID   int64
	TelegramName string
}

// Subject labels the caller for logs and audit trails. UserID cannot serve
// that purpose alone any more: a client or an advocate who came in through
// Telegram has no users row to name.
func (p Principal) Subject() string {
	switch {
	case p.UserID != "":
		return p.UserID
	case p.ClientID != "":
		return "client:" + p.ClientID
	case p.AdvocateID != "":
		return "advocate:" + p.AdvocateID
	default:
		return fmt.Sprintf("guest:tg%d", p.TelegramID)
	}
}

// UserLookup is the middleware's view of the user store — one method, declared
// where it is consumed.
type UserLookup interface {
	FindByID(ctx context.Context, id string) (user.User, error)
}

// LaunchVerifier is the middleware's view of tgauth.Verifier.
type LaunchVerifier interface {
	Verify(raw string) (tgauth.InitData, error)
}

// Authenticate identifies the caller and refuses the request unless the
// operation's scopes admit their role.
//
// launches and subjects may be nil — a deployment with no bot token configured
// cannot verify a launch, and then the Telegram scheme is simply unavailable
// rather than fatal at startup.
func Authenticate(
	tokens auth.TokenVerifier,
	users UserLookup,
	launches LaunchVerifier,
	subjects user.TelegramRepository,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes, requiresAuth := r.Context().Value(oapigen.BearerAuthScopes).([]string)
			if !requiresAuth {
				next.ServeHTTP(w, r)
				return
			}

			scheme, credential, found := splitAuthorization(r.Header.Get("Authorization"))
			if !found {
				log := logger.New(r.Context(), "middleware.auth")
				log.Warn().Str("reason", "missing_token").Msg("unauthorized")
				problem.Write(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			var (
				caller Principal
				ok     bool
			)
			switch {
			case strings.EqualFold(scheme, schemeBearer):
				caller, ok = principalFromToken(w, r, tokens, users, credential)
			case strings.EqualFold(scheme, schemeTelegram):
				caller, ok = principalFromLaunch(w, r, launches, subjects, credential)
			default:
				log := logger.New(r.Context(), "middleware.auth")
				log.Warn().Str("reason", "unsupported_scheme").Msg("unauthorized")
				problem.Write(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if !ok {
				return
			}

			if !allowedByScopes(caller.Role, scopes) {
				log := logger.New(r.Context(), "middleware.auth")
				log.Warn().Str("role", string(caller.Role)).Strs("scopes", scopes).Msg("forbidden")
				problem.Write(w, http.StatusForbidden, "forbidden")
				return
			}

			if h, ok := r.Context().Value(userIDHolderKey).(*userIDHolder); ok {
				h.id = caller.Subject()
			}
			ctx := context.WithValue(r.Context(), logger.ContextKeyUserID, caller.Subject())
			ctx = context.WithValue(ctx, principalKey, caller)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// principalFromToken resolves a password login or an API key, answering the
// client itself on failure.
func principalFromToken(
	w http.ResponseWriter,
	r *http.Request,
	tokens auth.TokenVerifier,
	users UserLookup,
	token string,
) (Principal, bool) {
	userID, err := tokens.Verify(r.Context(), token)
	if err != nil {
		log := logger.New(r.Context(), "middleware.auth")
		log.Warn().Str("reason", "verify_failed").Err(err).Msg("unauthorized")
		problem.Write(w, http.StatusUnauthorized, "unauthorized")
		return Principal{}, false
	}

	caller, err := users.FindByID(r.Context(), userID)
	if err != nil {
		// A token that verifies but names nobody is not an authorization
		// question — the account is gone, so the session is over.
		log := logger.New(r.Context(), "middleware.auth")
		log.Warn().Str("reason", "user_not_found").Err(err).Msg("unauthorized")
		problem.Write(w, http.StatusUnauthorized, "unauthorized")
		return Principal{}, false
	}

	return Principal{
		UserID:     userID,
		Role:       caller.Role,
		AdvocateID: caller.AdvocateID,
	}, true
}

// principalFromLaunch verifies a Mini App launch and resolves who it belongs
// to, answering the client itself on failure.
func principalFromLaunch(
	w http.ResponseWriter,
	r *http.Request,
	launches LaunchVerifier,
	subjects user.TelegramRepository,
	raw string,
) (Principal, bool) {
	log := logger.New(r.Context(), "middleware.auth")

	if launches == nil || subjects == nil {
		log.Warn().Str("reason", "telegram_auth_unconfigured").Msg("unauthorized")
		problem.Write(w, http.StatusUnauthorized, "unauthorized")
		return Principal{}, false
	}

	data, err := launches.Verify(raw)
	if err != nil {
		// Which check failed is useful to us and useful to someone probing, so
		// it is logged rather than returned.
		log.Warn().Str("reason", "launch_rejected").Err(err).Msg("unauthorized")
		problem.Write(w, http.StatusUnauthorized, "unauthorized")
		return Principal{}, false
	}

	subject, err := subjects.FindByTelegramID(r.Context(), data.User.ID)
	switch {
	case err == nil:
	case errors.Is(err, user.ErrNotFound):
		// A verified launch the CRM has never seen. Not an error: it is
		// someone's first visit, and the intake operation is what turns them
		// into a client.
		subject = user.TelegramSubject{Role: user.RoleGuest}
	default:
		// The credential is fine and the caller can do nothing about this, so
		// it must not read as "unauthorized".
		log.Error().Int64("telegram_id", data.User.ID).Err(err).Msg("telegram subject lookup failed")
		problem.Write(w, http.StatusInternalServerError, "internal error")
		return Principal{}, false
	}

	return Principal{
		Role:         subject.Role,
		AdvocateID:   subject.AdvocateID,
		ClientID:     subject.ClientID,
		TelegramID:   data.User.ID,
		TelegramName: telegramName(data.User),
	}, true
}

// telegramName spells a launch's user the way the bot already spells one when
// it writes clients.telegram_name — both write that column, so a client who
// arrives through the Mini App must not end up named differently from one the
// bot bound.
func telegramName(u tgauth.User) string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

// splitAuthorization splits an Authorization header into its scheme and
// credential. The scheme is returned as sent; callers compare it
// case-insensitively, as RFC 9110 requires.
func splitAuthorization(header string) (scheme, credential string, found bool) {
	scheme, credential, found = strings.Cut(strings.TrimSpace(header), " ")
	if !found {
		return "", "", false
	}
	credential = strings.TrimSpace(credential)
	if scheme == "" || credential == "" {
		return "", "", false
	}
	return scheme, credential, true
}

// allowedByScopes fails closed: an operation that names no role is admin-only.
// The alternative — an empty list meaning "anyone authenticated" — is how a
// forgotten or newly added endpoint silently becomes readable by an advocate,
// and that is precisely the leak this gate exists to prevent. It is also what
// keeps every existing operation shut to a client without one being revisited:
// reaching one takes writing "client" into its scopes.
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

// PrincipalFromContext is what a scoped handler filters on. The second return
// is false for unauthenticated routes, and a handler that needs a scope must
// refuse the request rather than fall back to "no filter".
//
// Presence is decided by the role, not by UserID: a Telegram caller is fully
// authenticated and has no users row.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok && p.Role != ""
}
