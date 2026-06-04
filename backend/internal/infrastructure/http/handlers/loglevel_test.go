package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apphealth "multiagent-seo/internal/application/health"
	domainhealth "multiagent-seo/internal/domain/health"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	httpMiddleware "multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/jwtauth"
	"multiagent-seo/pkg/config"
)

func newLogLevelRouter(jwtSvc *jwtauth.Service) http.Handler {
	healthHandler := handlers.NewHealthHandler(apphealth.NewService(domainhealth.NewService(stubRepo{})))
	server := handlers.NewServer(healthHandler, handlers.NewWordpressSitesHandler(nil), handlers.NewLoginHandler(nil), handlers.NewArticlesHandler(nil), handlers.NewLinkbuildingHandler(nil, nil, nil), handlers.NewApiTokensHandler(nil))
	return apihttp.NewRouter(
		config.ServerConfig{BasePath: "/", CORSAllowedOrigins: []string{"http://localhost:3000"}},
		server,
		httpMiddleware.BearerAuth(jwtSvc),
	)
}

func TestDebugLogLevel_RequiresAuth(t *testing.T) {
	jwtSvc := jwtauth.New("test-secret", time.Hour)
	srv := httptest.NewServer(newLogLevelRouter(jwtSvc))
	defer srv.Close()

	for _, m := range []string{http.MethodGet, http.MethodPut} {
		req, _ := http.NewRequest(m, srv.URL+"/debug/log-level", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", m, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s /debug/log-level without token = %d, want 401", m, resp.StatusCode)
		}
	}

	token, _, err := jwtSvc.Issue(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/debug/log-level", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authed GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("authed GET /debug/log-level = %d, want 200", resp.StatusCode)
	}
}
