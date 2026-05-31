package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	appauth "multiagent-seo/internal/application/auth"
	apphealth "multiagent-seo/internal/application/health"
	domainhealth "multiagent-seo/internal/domain/health"
	domainuser "multiagent-seo/internal/domain/user"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/infrastructure/jwtauth"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/config"
)

type fakeUserRepo struct {
	users map[string]domainuser.User
}

func (r fakeUserRepo) FindByEmail(_ context.Context, email string) (domainuser.User, error) {
	u, ok := r.users[email]
	if !ok {
		return domainuser.User{}, domainuser.ErrNotFound
	}
	return u, nil
}

func (r fakeUserRepo) List(context.Context) ([]domainuser.User, error) {
	out := make([]domainuser.User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, u)
	}
	return out, nil
}

func newAuthRouter(t *testing.T) http.Handler {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	repo := fakeUserRepo{users: map[string]domainuser.User{
		"user@example.com": {ID: uuid.New(), Email: "user@example.com", PasswordHash: string(hash)},
	}}
	authSvc := appauth.NewService(repo, jwtauth.New("test-secret", time.Hour))

	healthHandler := handlers.NewHealthHandler(apphealth.NewService(domainhealth.NewService(stubRepo{})))
	server := handlers.NewServer(healthHandler, handlers.NewWordpressSitesHandler(nil), handlers.NewLoginHandler(authSvc), handlers.NewArticlesHandler(nil), handlers.NewLinkbuildingHandler(nil, nil), handlers.NewApiTokensHandler(nil))
	return apihttp.NewRouter(config.ServerConfig{
		Host:               "localhost",
		Port:               "0",
		BasePath:           "/",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}, server, nil)
}

func TestLogin_Success(t *testing.T) {
	router := newAuthRouter(t)
	rec := doJSON(t, router, http.MethodPost, "/auth/login", oapigen.LoginRequest{
		Email:    "user@example.com",
		Password: "correct-horse",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body oapigen.LoginResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Token == "" {
		t.Errorf("expected non-empty token")
	}
	if body.ExpiresAt.IsZero() {
		t.Errorf("expected expiresAt to be set")
	}
}

func TestLogin_BadCredentials(t *testing.T) {
	router := newAuthRouter(t)
	rec := doJSON(t, router, http.MethodPost, "/auth/login", oapigen.LoginRequest{
		Email:    "user@example.com",
		Password: "wrong",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	router := newAuthRouter(t)
	rec := doJSON(t, router, http.MethodPost, "/auth/login", oapigen.LoginRequest{
		Email:    "nobody@example.com",
		Password: "correct-horse",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestLogin_InvalidBody(t *testing.T) {
	router := newAuthRouter(t)
	rec := doJSON(t, router, http.MethodPost, "/auth/login", oapigen.LoginRequest{
		Email:    "not-an-email",
		Password: "whatever",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}
