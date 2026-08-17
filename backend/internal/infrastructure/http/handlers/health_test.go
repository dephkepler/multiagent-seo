package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apphealth "multiagent-seo/internal/application/health"
	domainhealth "multiagent-seo/internal/domain/health"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/config"
)

type stubRepo struct{ err error }

func (s stubRepo) Ping(context.Context) error { return s.err }

func newRouter(pingErr error) http.Handler {
	svc := apphealth.NewService(domainhealth.NewService(stubRepo{err: pingErr}))
	server := handlers.NewServer(handlers.NewHealthHandler(svc), handlers.NewWordpressSitesHandler(nil), handlers.NewLoginHandler(nil), handlers.NewArticlesHandler(nil), handlers.NewLinkbuildingHandler(nil), handlers.NewApiTokensHandler(nil), handlers.NewEmailScrapeHandler(nil), handlers.NewLeadStatsHandler(nil), handlers.NewClientSegmentsHandler(nil), handlers.NewClientDetailHandler(nil))
	return apihttp.NewRouter(config.ServerConfig{
		Host:               "localhost",
		Port:               "0",
		BasePath:           "/",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}, server, nil)
}

func TestGetHealthz_Healthy(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body oapigen.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != oapigen.Ok {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if body.Time.IsZero() {
		t.Errorf("time should be set")
	}
}

func TestGetHealthz_Degraded(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(context.DeadlineExceeded).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body oapigen.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != oapigen.Degraded {
		t.Errorf("status = %q, want degraded", body.Status)
	}
}
