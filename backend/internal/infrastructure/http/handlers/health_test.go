package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	server := handlers.NewServer(handlers.NewHealthHandler(svc), handlers.NewWordpressSitesHandler(nil), handlers.NewLoginHandler(nil))
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

func TestGetHealthz_Load(t *testing.T) {
	if testing.Short() {
		t.Skip("load test skipped in -short")
	}
	srv := httptest.NewServer(newRouter(nil))
	defer srv.Close()

	const (
		workers      = 50
		perWorker    = 200
		totalReqs    = workers * perWorker
	)

	var ok, failed int64
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()

	for range workers {
		wg.Go(func() {
			for range perWorker {
				resp, err := client.Get(srv.URL + "/healthz")
				if err != nil || resp.StatusCode != http.StatusOK {
					atomic.AddInt64(&failed, 1)
					if resp != nil {
						resp.Body.Close()
					}
					continue
				}
				resp.Body.Close()
				atomic.AddInt64(&ok, 1)
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	if failed > 0 {
		t.Errorf("%d/%d requests failed under load", failed, totalReqs)
	}
	t.Logf("load: %d reqs in %s = %.0f req/s, %.3f ms/req avg",
		totalReqs, elapsed.Round(time.Millisecond),
		float64(totalReqs)/elapsed.Seconds(),
		float64(elapsed.Microseconds())/float64(totalReqs)/1000)
}

func BenchmarkGetHealthz(b *testing.B) {
	router := newRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	b.ReportAllocs()
	for b.Loop() {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d", rec.Code)
		}
	}
}
