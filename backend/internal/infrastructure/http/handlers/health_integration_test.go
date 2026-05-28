//go:build integration

// Run with a reachable Postgres (CF_DB_* env), e.g. against devops/compose:
//   CF_DB_PORT=55433 CF_DB_USER=contentflow CF_DB_PASSWORD=contentflow \
//   CF_DB_NAME=contentflow go test -tags integration ./internal/infrastructure/http/handlers/
package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apphealth "contentflow/internal/application/health"
	domainhealth "contentflow/internal/domain/health"
	"contentflow/internal/infrastructure/db"
	apihttp "contentflow/internal/infrastructure/http"
	"contentflow/internal/infrastructure/http/handlers"
	"contentflow/internal/infrastructure/persistence/postgres"
	"contentflow/internal/oapigen"
	"contentflow/pkg/config"
)

func TestHealthz_Integration(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skipf("config load failed: %v", err)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		t.Skipf("no reachable database: %v", err)
	}
	defer pool.Close()

	repo := postgres.NewHealthRepository(pool)
	svc := apphealth.NewService(domainhealth.NewService(repo))
	router := apihttp.NewRouter(cfg.Server, handlers.NewServer(handlers.NewHealthHandler(svc)))

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
