package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	apparticles "multiagent-seo/internal/application/articles"
	apphealth "multiagent-seo/internal/application/health"
	domainhealth "multiagent-seo/internal/domain/health"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/config"
)

type capturingBatchService struct {
	fakeGenerateService

	batchIDs      []int64
	batchErr      error
	batchCalls    int
	capturedCount int
	capturedShare apparticles.GenerateRequest
}

func (s *capturingBatchService) GenerateBatch(_ context.Context, count int, shared apparticles.GenerateRequest) ([]int64, error) {
	s.batchCalls++
	s.capturedCount = count
	s.capturedShare = shared
	if s.batchErr != nil {
		return nil, s.batchErr
	}
	return s.batchIDs, nil
}

func ptrTo[T any](v T) *T { return &v }

func TestArticles_GenerateBatch_ForcesImageAttributionFalse(t *testing.T) {
	svc := &capturingBatchService{batchIDs: []int64{10, 11}}
	router := newArticlesRouter2(svc)

	rec := doJSON(t, router, http.MethodPost, "/generate/batch", oapigen.GenerateBatchRequest{
		SiteId: uuid.New(),
		Count:  5,
	})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
	if svc.capturedShare.IncludeImageAttribution == nil {
		t.Fatal("IncludeImageAttribution is nil, want non-nil false")
	}
	if *svc.capturedShare.IncludeImageAttribution {
		t.Fatal("IncludeImageAttribution = true, want false (Pexels-leak guard)")
	}
}

func TestArticles_GenerateBatch_RejectsZeroSiteID(t *testing.T) {
	svc := &capturingBatchService{batchIDs: []int64{1}}
	router := newArticlesRouter2(svc)

	rec := doJSON(t, router, http.MethodPost, "/generate/batch", oapigen.GenerateBatchRequest{
		SiteId: uuid.Nil,
		Count:  5,
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	if svc.batchCalls != 0 {
		t.Fatalf("GenerateBatch called %d times, want 0", svc.batchCalls)
	}
}

func TestArticles_GenerateBatch_AcceptedBodyShape(t *testing.T) {
	svc := &capturingBatchService{batchIDs: []int64{101, 102, 103}}
	router := newArticlesRouter2(svc)

	rec := doJSON(t, router, http.MethodPost, "/generate/batch", oapigen.GenerateBatchRequest{
		SiteId: uuid.New(),
		Count:  3,
	})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
	var decoded oapigen.GenerateBatchAccepted
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, rec.Body.String())
	}
	if decoded.Count != 3 {
		t.Fatalf("Count = %d, want 3", decoded.Count)
	}
	if len(decoded.ArticleIds) != 3 || decoded.ArticleIds[0] != 101 || decoded.ArticleIds[1] != 102 || decoded.ArticleIds[2] != 103 {
		t.Fatalf("ArticleIds = %v, want [101 102 103]", decoded.ArticleIds)
	}
	if decoded.Count != len(decoded.ArticleIds) {
		t.Fatalf("Count %d != len(ArticleIds) %d", decoded.Count, len(decoded.ArticleIds))
	}
}

func TestArticles_GenerateBatch_RejectsInvalidCount(t *testing.T) {
	cases := []struct {
		name  string
		count int
	}{
		{"zero", 0},
		{"negative", -1},
		{"over_max", 101},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &capturingBatchService{batchIDs: []int64{1}}
			router := newArticlesRouter2(svc)

			rec := doJSON(t, router, http.MethodPost, "/generate/batch", oapigen.GenerateBatchRequest{
				SiteId: uuid.New(),
				Count:  tc.count,
			})

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
			}
			if svc.batchCalls != 0 {
				t.Fatalf("GenerateBatch called %d times, want 0", svc.batchCalls)
			}
		})
	}
}

func TestArticles_GenerateBatch_InvalidBody(t *testing.T) {
	svc := &capturingBatchService{batchIDs: []int64{1}}
	router := newArticlesRouter2(svc)

	req := httptest.NewRequest(http.MethodPost, "/generate/batch", strings.NewReader(`{"count":"abc"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid request body") {
		t.Fatalf("body = %s, want problem detail %q", rec.Body.String(), "invalid request body")
	}
	if svc.batchCalls != 0 {
		t.Fatalf("GenerateBatch called %d times, want 0", svc.batchCalls)
	}
}

func TestArticles_GenerateBatch_MapsSharedSettings(t *testing.T) {
	svc := &capturingBatchService{batchIDs: []int64{1, 2}}
	router := newArticlesRouter2(svc)
	sid := uuid.New()

	rec := doJSON(t, router, http.MethodPost, "/generate/batch", oapigen.GenerateBatchRequest{
		SiteId:        sid,
		Count:         2,
		AutoPublish:   ptrTo(true),
		IncludeImages: ptrTo(true),
		MinWords:      ptrTo(800),
		MaxWords:      ptrTo(1500),
		MaxTokens:     ptrTo(4000),
		MaxCycles:     ptrTo(3),
		Language:      ptrTo("en"),
		Provider:      ptrTo("anthropic"),
		Model:         ptrTo("claude"),
		AiThreshold:   ptrTo(float32(0.7)),
	})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
	got := svc.capturedShare
	if got.SiteID != sid {
		t.Fatalf("SiteID = %v, want %v", got.SiteID, sid)
	}
	if !got.AutoPublish {
		t.Fatal("AutoPublish = false, want true")
	}
	if got.IncludeImages == nil || !*got.IncludeImages {
		t.Fatalf("IncludeImages = %v, want pointer to true", got.IncludeImages)
	}
	if got.MinWords != 800 {
		t.Fatalf("MinWords = %d, want 800", got.MinWords)
	}
	if got.MaxWords != 1500 {
		t.Fatalf("MaxWords = %d, want 1500", got.MaxWords)
	}
	if got.MaxTokens != 4000 {
		t.Fatalf("MaxTokens = %d, want 4000", got.MaxTokens)
	}
	if got.MaxCycles != 3 {
		t.Fatalf("MaxCycles = %d, want 3", got.MaxCycles)
	}
	if got.Language != "en" {
		t.Fatalf("Language = %q, want en", got.Language)
	}
	if got.Provider != "anthropic" {
		t.Fatalf("Provider = %q, want anthropic", got.Provider)
	}
	if got.Model != "claude" {
		t.Fatalf("Model = %q, want claude", got.Model)
	}
	if got.AIThreshold != float64(float32(0.7)) {
		t.Fatalf("AIThreshold = %v, want %v", got.AIThreshold, float64(float32(0.7)))
	}
	if svc.capturedCount != 2 {
		t.Fatalf("count passed to service = %d, want 2", svc.capturedCount)
	}
}

func TestArticles_GenerateBatch_ServiceErrorMaps500(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		svc := &capturingBatchService{batchErr: errors.New("boom")}
		router := newArticlesRouter2(svc)
		rec := doJSON(t, router, http.MethodPost, "/generate/batch", oapigen.GenerateBatchRequest{
			SiteId: uuid.New(),
			Count:  2,
		})
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (body=%s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("no_cluster", func(t *testing.T) {
		svc := &capturingBatchService{batchErr: fmt.Errorf("batch: %w", apparticles.ErrNoCluster)}
		router := newArticlesRouter2(svc)
		rec := doJSON(t, router, http.MethodPost, "/generate/batch", oapigen.GenerateBatchRequest{
			SiteId: uuid.New(),
			Count:  2,
		})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
		}
	})
}

func newArticlesRouter2(svc *capturingBatchService) http.Handler {
	healthHandler := handlers.NewHealthHandler(apphealth.NewService(domainhealth.NewService(stubRepo{})))
	server := handlers.NewServer(handlers.Deps{
		Health:   healthHandler,
		Articles: handlers.NewArticlesHandler(svc),
	})
	return apihttp.NewRouter(config.ServerConfig{
		Host:               "localhost",
		Port:               "0",
		BasePath:           "/",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}, server, nil)
}
