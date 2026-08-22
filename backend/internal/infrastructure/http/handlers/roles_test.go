package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	domainadvocateview "multiagent-seo/internal/domain/advocateview"
	domainuser "multiagent-seo/internal/domain/user"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	httpMiddleware "multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/jwtauth"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/config"
)

// fakeAdvocateView answers /my with one case, enough to tell "the gate let me
// through" from "the gate refused me".
type fakeAdvocateView struct {
	advocateID string
}

func (f fakeAdvocateView) Cases(_ context.Context, advocateID string) ([]domainadvocateview.Case, error) {
	if advocateID != f.advocateID {
		return nil, domainadvocateview.ErrNotFound
	}
	return []domainadvocateview.Case{{ID: "case-1", ClientName: "Клієнт", Fee: 10000, Paid: 4000}}, nil
}

func (f fakeAdvocateView) Clients(context.Context, string) ([]domainadvocateview.Client, error) {
	return nil, nil
}

func (f fakeAdvocateView) Client(context.Context, string, string) (domainadvocateview.Card, error) {
	return domainadvocateview.Card{}, domainadvocateview.ErrNotFound
}

func (f fakeAdvocateView) AddNote(context.Context, string, string, string, string) (domainadvocateview.Note, error) {
	return domainadvocateview.Note{}, domainadvocateview.ErrNotFound
}

func (f fakeAdvocateView) SetCaseStatus(context.Context, string, string, string) error {
	return domainadvocateview.ErrNotFound
}

func (f fakeAdvocateView) Settlement(_ context.Context, advocateID string) (domainadvocateview.Settlement, error) {
	return domainadvocateview.Settlement{AdvocateID: advocateID, Collected: 4000}, nil
}

func (f fakeAdvocateView) Stats(context.Context, string) (domainadvocateview.Stats, error) {
	return domainadvocateview.Stats{Cases: 1}, nil
}

type roleRouter struct {
	handler       http.Handler
	adminToken    string
	advocateToken string
}

func newRoleRouter(t *testing.T) roleRouter {
	t.Helper()

	adminID, advocateUserID, advocateID := uuid.New(), uuid.New(), uuid.New().String()
	users := adminLookup{
		adminID.String(): {ID: adminID, Email: "admin@example.com", Role: domainuser.RoleAdmin},
		advocateUserID.String(): {
			ID:         advocateUserID,
			Email:      "advocate@example.com",
			Role:       domainuser.RoleAdvocate,
			AdvocateID: advocateID,
		},
	}

	jwtSvc := jwtauth.New("test-secret", time.Hour)
	adminToken, _, err := jwtSvc.Issue(context.Background(), adminID.String())
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	advocateToken, _, err := jwtSvc.Issue(context.Background(), advocateUserID.String())
	if err != nil {
		t.Fatalf("issue advocate token: %v", err)
	}

	server := handlers.NewServer(handlers.Deps{
		My: handlers.NewMyHandler(fakeAdvocateView{advocateID: advocateID}),
	})
	router := apihttp.NewRouter(
		config.ServerConfig{BasePath: "/", CORSAllowedOrigins: []string{"http://localhost:3000"}},
		server,
		httpMiddleware.Authenticate(jwtSvc, users, nil, nil),
	)
	return roleRouter{handler: router, adminToken: adminToken, advocateToken: advocateToken}
}

func (r roleRouter) do(t *testing.T, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

// adminOnlyRoutes is the list the leak would show up on: the firm's margin, the
// expense ledger with every contractor's name, the plaintext password vault, all
// clients including the encrypted personal data, the user list, and the rate an
// advocate would otherwise be able to raise for themselves.
var adminOnlyRoutes = []struct {
	method string
	path   string
	body   any
}{
	{http.MethodGet, "/finance/report?from=2026-01-01&to=2026-01-31", nil},
	{http.MethodGet, "/finance/expenses", nil},
	{http.MethodGet, "/finance/settlement?from=2026-01-01&to=2026-01-31", nil},
	{http.MethodGet, "/vault-entries", nil},
	{http.MethodGet, "/vault-groups", nil},
	{http.MethodGet, "/clients", nil},
	{http.MethodGet, "/users", nil},
	{http.MethodGet, "/api-tokens", nil},
	{http.MethodPatch, "/finance/advocate-rates/" + uuid.New().String(), oapigen.SetAdvocateRateRequest{CommissionPercent: 99}},
}

func TestAdvocateTokenIsRefusedOnAdminRoutes(t *testing.T) {
	router := newRoleRouter(t)

	for _, route := range adminOnlyRoutes {
		rec := router.do(t, router.advocateToken, route.method, route.path, route.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as advocate = %d, want 403 (body=%s)", route.method, route.path, rec.Code, rec.Body.String())
		}
	}
}

// The admin must not be locked out by the same gate. The services are not wired
// in this router, so "allowed" shows up as 503 rather than 200 — what matters is
// that the request reached the handler at all instead of stopping at 403.
func TestAdminTokenPassesTheRoleGate(t *testing.T) {
	router := newRoleRouter(t)

	for _, route := range adminOnlyRoutes {
		rec := router.do(t, router.adminToken, route.method, route.path, route.body)
		if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s as admin = %d, want the gate to let it through", route.method, route.path, rec.Code)
		}
	}
}

func TestAdvocateTokenReachesOwnSection(t *testing.T) {
	router := newRoleRouter(t)

	for _, path := range []string{"/my/cases", "/my/clients", "/my/settlement", "/my/stats"} {
		rec := router.do(t, router.advocateToken, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s as advocate = %d, want 200 (body=%s)", path, rec.Code, rec.Body.String())
		}
	}

	var list oapigen.MyCaseList
	rec := router.do(t, router.advocateToken, http.MethodGet, "/my/cases", nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode /my/cases: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Id != "case-1" {
		t.Fatalf("items = %+v, want the advocate's own case", list.Items)
	}
	if list.TotalOwed != 6000 {
		t.Errorf("total_owed = %v, want 6000", list.TotalOwed)
	}
}

// An admin login is allowed onto /my routes on purpose — so the section can be
// inspected without a second account — but it has no roster row, and answering
// with everything would turn the advocate section into the admin one. It says
// so instead.
func TestAdminOnAdvocateSectionIsRefusedNotWidened(t *testing.T) {
	router := newRoleRouter(t)

	rec := router.do(t, router.adminToken, http.MethodGet, "/my/cases", nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("GET /my/cases as admin = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestNoTokenIsUnauthorizedEverywhere(t *testing.T) {
	router := newRoleRouter(t)

	for _, path := range []string{"/my/cases", "/finance/expenses", "/vault-entries"} {
		rec := router.do(t, "", http.MethodGet, path, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without a token = %d, want 401", path, rec.Code)
		}
	}
}

// A token that verifies but names a user who no longer exists is a dead
// session, not an authorization question — deleting an account has to end it.
func TestTokenForDeletedUserIsUnauthorized(t *testing.T) {
	jwtSvc := jwtauth.New("test-secret", time.Hour)
	token, _, err := jwtSvc.Issue(context.Background(), uuid.New().String())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	server := handlers.NewServer(handlers.Deps{})
	router := apihttp.NewRouter(
		config.ServerConfig{BasePath: "/", CORSAllowedOrigins: []string{"http://localhost:3000"}},
		server,
		httpMiddleware.Authenticate(jwtSvc, adminLookup{}, nil, nil),
	)

	req := httptest.NewRequest(http.MethodGet, "/vault-entries", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (body=%s)", rec.Code, rec.Body.String())
	}
}
