package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"multiagent-seo/internal/domain/user"
	"multiagent-seo/internal/infrastructure/tgauth"
	"multiagent-seo/internal/oapigen"
)

const (
	clientTelegramID   int64 = 111
	advocateTelegramID int64 = 222
	strangerTelegramID int64 = 333
)

type stubTokens map[string]string

func (s stubTokens) Verify(_ context.Context, token string) (string, error) {
	id, ok := s[token]
	if !ok {
		return "", errors.New("bad token")
	}
	return id, nil
}

type stubUsers map[string]user.User

func (s stubUsers) FindByID(_ context.Context, id string) (user.User, error) {
	u, ok := s[id]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return u, nil
}

// stubLaunches answers with one fixed launch, or an error for anything else, so
// a test picks a path by the credential it sends rather than by rebuilding the
// middleware.
type stubLaunches struct {
	accept string
	data   tgauth.InitData
}

func (s stubLaunches) Verify(raw string) (tgauth.InitData, error) {
	if raw != s.accept {
		return tgauth.InitData{}, tgauth.ErrBadHash
	}
	return s.data, nil
}

type stubSubjects struct {
	byTelegramID map[int64]user.TelegramSubject
	err          error
}

func (s stubSubjects) FindByTelegramID(_ context.Context, id int64) (user.TelegramSubject, error) {
	if s.err != nil {
		return user.TelegramSubject{}, s.err
	}
	subject, ok := s.byTelegramID[id]
	if !ok {
		return user.TelegramSubject{}, user.ErrNotFound
	}
	return subject, nil
}

func launchFor(id int64) tgauth.InitData {
	return tgauth.InitData{User: tgauth.User{ID: id, Username: "petro"}}
}

// probe stands in for a handler: it records the principal the middleware
// resolved, which is the only way to see a successful authentication without an
// endpoint that admits the role under test.
type probe struct {
	principal Principal
	present   bool
}

func (p *probe) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	p.principal, p.present = PrincipalFromContext(r.Context())
}

type callOptions struct {
	scopes     []string
	authHeader string
	launches   LaunchVerifier
	subjects   user.TelegramRepository
}

// call runs one request through the middleware. Scopes arrive the way oapigen
// puts them there, since that context value is what the gate reads.
func call(t *testing.T, opts callOptions) (*httptest.ResponseRecorder, *probe) {
	t.Helper()

	tokens := stubTokens{"admin-token": "user-1"}
	users := stubUsers{"user-1": {Role: user.RoleAdmin}}

	target := &probe{}
	handler := Authenticate(tokens, users, opts.launches, opts.subjects)(target)

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	if opts.authHeader != "" {
		req.Header.Set("Authorization", opts.authHeader)
	}
	ctx := context.WithValue(req.Context(), oapigen.BearerAuthScopes, opts.scopes)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(ctx))
	return rec, target
}

func telegramCall(t *testing.T, scopes []string, telegramID int64, subjects map[int64]user.TelegramSubject) (*httptest.ResponseRecorder, *probe) {
	t.Helper()
	return call(t, callOptions{
		scopes:     scopes,
		authHeader: "tma raw-init-data",
		launches:   stubLaunches{accept: "raw-init-data", data: launchFor(telegramID)},
		subjects:   stubSubjects{byTelegramID: subjects},
	})
}

var knownSubjects = map[int64]user.TelegramSubject{
	clientTelegramID:   {Role: user.RoleClient, ClientID: "client-uuid"},
	advocateTelegramID: {Role: user.RoleAdvocate, AdvocateID: "advocate-uuid"},
}

func TestLaunchAuthenticatesAKnownClient(t *testing.T) {
	rec, target := telegramCall(t, []string{"client"}, clientTelegramID, knownSubjects)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	if !target.present {
		t.Fatal("handler saw no principal")
	}
	if target.principal.Role != user.RoleClient {
		t.Errorf("role = %q, want client", target.principal.Role)
	}
	if target.principal.ClientID != "client-uuid" {
		t.Errorf("client id = %q, want client-uuid", target.principal.ClientID)
	}
	if target.principal.UserID != "" {
		t.Errorf("user id = %q, want empty — a client has no users row", target.principal.UserID)
	}
	// The intake flow binds the new client to this chat, so losing it here
	// would mean a client who can never be recognised again.
	if target.principal.TelegramID != clientTelegramID {
		t.Errorf("telegram id = %d, want %d", target.principal.TelegramID, clientTelegramID)
	}
	if target.principal.TelegramName != "@petro" {
		t.Errorf("telegram name = %q, want @petro", target.principal.TelegramName)
	}
}

func TestLaunchAuthenticatesAnAdvocateWithoutAPassword(t *testing.T) {
	rec, target := telegramCall(t, []string{"advocate"}, advocateTelegramID, knownSubjects)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if target.principal.Role != user.RoleAdvocate {
		t.Errorf("role = %q, want advocate", target.principal.Role)
	}
	if target.principal.AdvocateID != "advocate-uuid" {
		t.Errorf("advocate id = %q, want advocate-uuid", target.principal.AdvocateID)
	}
}

func TestUnknownLaunchBecomesAGuest(t *testing.T) {
	rec, target := telegramCall(t, []string{"guest"}, strangerTelegramID, knownSubjects)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a first-time visitor is not an error", rec.Code)
	}
	if target.principal.Role != user.RoleGuest {
		t.Errorf("role = %q, want guest", target.principal.Role)
	}
	if target.principal.ClientID != "" {
		t.Errorf("client id = %q, want empty", target.principal.ClientID)
	}
	if target.principal.TelegramID != strangerTelegramID {
		t.Errorf("telegram id = %d, want %d", target.principal.TelegramID, strangerTelegramID)
	}
}

func TestGuestIsRefusedWhereAClientIsRequired(t *testing.T) {
	rec, target := telegramCall(t, []string{"client"}, strangerTelegramID, knownSubjects)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if target.present {
		t.Error("handler ran for a guest on a client-only operation")
	}
}

// The gate fails closed, which is what keeps every operation written before
// clients existed shut to them without being revisited.
func TestTelegramCallersAreRefusedByAnOperationWithNoScopes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		telegramID int64
	}{
		{"client", clientTelegramID},
		{"advocate", advocateTelegramID},
		{"guest", strangerTelegramID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, target := telegramCall(t, nil, tc.telegramID, knownSubjects)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
			if target.present {
				t.Error("handler ran on an admin-only operation")
			}
		})
	}
}

func TestRejectedLaunchIsUnauthorized(t *testing.T) {
	rec, target := call(t, callOptions{
		scopes:     []string{"client"},
		authHeader: "tma tampered",
		launches:   stubLaunches{accept: "raw-init-data", data: launchFor(clientTelegramID)},
		subjects:   stubSubjects{byTelegramID: knownSubjects},
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if target.present {
		t.Error("handler ran on an unverified launch")
	}
}

// A lookup that failed says nothing about the credential, and telling the app
// "unauthorized" would send the client into "reopen the app" forever.
func TestSubjectLookupFailureIsInternal(t *testing.T) {
	rec, _ := call(t, callOptions{
		scopes:     []string{"client"},
		authHeader: "tma raw-init-data",
		launches:   stubLaunches{accept: "raw-init-data", data: launchFor(clientTelegramID)},
		subjects:   stubSubjects{err: errors.New("connection refused")},
	})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestTelegramSchemeIsUnavailableWithoutABotToken(t *testing.T) {
	rec, _ := call(t, callOptions{
		scopes:     []string{"client"},
		authHeader: "tma raw-init-data",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestUnknownSchemeIsUnauthorized(t *testing.T) {
	for _, header := range []string{"Basic dXNlcjpwYXNz", "raw-init-data", "tma", "Bearer"} {
		rec, _ := call(t, callOptions{scopes: []string{"admin"}, authHeader: header})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status = %d, want 401", header, rec.Code)
		}
	}
}

// The password path must be untouched by the dispatch, and the scheme is
// case-insensitive per RFC 9110.
func TestPasswordLoginStillAuthenticates(t *testing.T) {
	for _, header := range []string{"Bearer admin-token", "bearer admin-token"} {
		rec, target := call(t, callOptions{scopes: []string{"admin"}, authHeader: header})
		if rec.Code != http.StatusOK {
			t.Fatalf("header %q: status = %d, want 200", header, rec.Code)
		}
		if target.principal.UserID != "user-1" || target.principal.Role != user.RoleAdmin {
			t.Errorf("header %q: principal = %+v", header, target.principal)
		}
		if target.principal.TelegramID != 0 {
			t.Errorf("header %q: telegram id = %d, want 0", header, target.principal.TelegramID)
		}
	}
}

func TestSubjectNamesTheCallerForLogs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		principal Principal
		want      string
	}{
		{"password login", Principal{UserID: "user-1"}, "user-1"},
		{"client", Principal{Role: user.RoleClient, ClientID: "c-1"}, "client:c-1"},
		{"advocate", Principal{Role: user.RoleAdvocate, AdvocateID: "a-1"}, "advocate:a-1"},
		{"guest", Principal{Role: user.RoleGuest, TelegramID: 42}, "guest:tg42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.principal.Subject(); got != tc.want {
				t.Errorf("Subject() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUnauthenticatedRouteSkipsTheGateEntirely(t *testing.T) {
	target := &probe{}
	handler := Authenticate(stubTokens{}, stubUsers{}, nil, nil)(target)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if target.present {
		t.Error("a public route must not carry a principal")
	}
}
