//go:build integration

package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	appauth "multiagent-seo/internal/application/auth"
	apphealth "multiagent-seo/internal/application/health"
	domainhealth "multiagent-seo/internal/domain/health"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/infrastructure/jwtauth"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/internal/testsupport"
	"multiagent-seo/pkg/config"
)

const itAuthSecret = "test-secret"

func itAuthSeedUser(t *testing.T, pool *pgxpool.Pool, email, password string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = pool.Exec(context.Background(),
		`INSERT INTO users (email, password_hash) VALUES (@email, @hash)`,
		pgx.NamedArgs{"email": email, "hash": string(hash)})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func itAuthServer(t *testing.T, pool *pgxpool.Pool) *httptest.Server {
	t.Helper()
	jwtSvc := jwtauth.New(itAuthSecret, time.Hour)
	authSvc := appauth.NewService(postgres.NewUserRepository(pool), jwtSvc)

	healthHandler := handlers.NewHealthHandler(apphealth.NewService(domainhealth.NewService(stubRepo{})))
	server := handlers.NewServer(
		healthHandler,
		handlers.NewWordpressSitesHandler(nil),
		handlers.NewLoginHandler(authSvc),
		handlers.NewArticlesHandler(nil),
		handlers.NewLinkbuildingHandler(nil),
		handlers.NewApiTokensHandler(nil), handlers.NewEmailScrapeHandler(nil), handlers.NewLeadStatsHandler(nil),
		handlers.NewClientSegmentsHandler(nil),
		handlers.NewClientDetailHandler(nil),
		handlers.NewVaultHandler(nil),
		handlers.NewFinanceHandler(nil),
	)
	router := apihttp.NewRouter(config.ServerConfig{
		BasePath:           "/",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}, server, nil)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func itAuthPostLogin(t *testing.T, srv *httptest.Server, req oapigen.LoginRequest) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(req); err != nil {
		t.Fatalf("encode login: %v", err)
	}
	resp, err := http.Post(srv.URL+"/auth/login", "application/json", &buf)
	if err != nil {
		t.Fatalf("POST /auth/login: %v", err)
	}
	return resp
}

func TestAuthLogin_Integration(t *testing.T) {
	pool := testsupport.NewTestDB(t, baseConnStr)
	const email, password = "user@example.com", "correct-horse-battery"
	itAuthSeedUser(t, pool, email, password)

	srv := itAuthServer(t, pool)

	t.Run("valid credentials yield a verifiable token", func(t *testing.T) {
		resp := itAuthPostLogin(t, srv, oapigen.LoginRequest{Email: email, Password: password})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var body oapigen.LoginResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode login response: %v", err)
		}
		if body.Token == "" {
			t.Fatal("expected non-empty token")
		}

		verifier := jwtauth.New(itAuthSecret, time.Hour)
		subject, err := verifier.Verify(context.Background(), body.Token)
		if err != nil {
			t.Fatalf("verify token: %v", err)
		}
		if subject == "" {
			t.Error("expected token subject to carry the user id")
		}
	})

	t.Run("wrong password is rejected", func(t *testing.T) {
		resp := itAuthPostLogin(t, srv, oapigen.LoginRequest{Email: email, Password: "wrong-password"})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})
}
