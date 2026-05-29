//go:build integration

package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apphealth "multiagent-seo/internal/application/health"
	domainhealth "multiagent-seo/internal/domain/health"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/internal/testsupport"
	"multiagent-seo/pkg/config"
)

func TestHealthz_Integration(t *testing.T) {
	pool := testsupport.NewTestDB(t, baseConnStr)

	repo := postgres.NewHealthRepository(pool)
	svc := apphealth.NewService(domainhealth.NewService(repo))
	router := apihttp.NewRouter(
		config.ServerConfig{BasePath: "/", CORSAllowedOrigins: []string{"http://localhost:3000"}},
		handlers.NewServer(handlers.NewHealthHandler(svc), handlers.NewWordpressSitesHandler(nil), handlers.NewLoginHandler(nil), handlers.NewArticlesHandler(nil), handlers.NewLinkbuildingHandler(nil), handlers.NewApiTokensHandler(nil)),
		nil,
	)

	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 against live DB", resp.StatusCode)
	}
	var body oapigen.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != oapigen.Ok {
		t.Errorf("status = %q, want ok", body.Status)
	}
}
