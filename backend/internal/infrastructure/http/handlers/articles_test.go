package handlers_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	appgen "multiagent-seo/internal/application/generate"
	apphealth "multiagent-seo/internal/application/health"
	"multiagent-seo/internal/domain/generate"
	domainhealth "multiagent-seo/internal/domain/health"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/config"
)

type fakeGenerateService struct {
	generated generate.Article
	genErr    error
	arts      []generate.Article
	getErr    error
	pubErr    error
}

func (s *fakeGenerateService) Generate(_ context.Context, req appgen.GenerateRequest) (appgen.GenerateResult, error) {
	if s.genErr != nil {
		return appgen.GenerateResult{}, s.genErr
	}
	return appgen.GenerateResult{
		Article:        generate.Article{ID: 1, Keyword: req.Keyword, SiteID: req.SiteID, Status: generate.StatusGenerating},
		TargetKeywords: []string{req.Keyword},
	}, nil
}

func (s *fakeGenerateService) Publish(context.Context, int64) (generate.Article, error) {
	if s.pubErr != nil {
		return generate.Article{}, s.pubErr
	}
	return generate.Article{ID: 1, Status: generate.StatusPublished}, nil
}

func (s *fakeGenerateService) List(context.Context) ([]generate.Article, error) {
	return s.arts, nil
}

func (s *fakeGenerateService) Get(context.Context, int64) (generate.Article, error) {
	if s.getErr != nil {
		return generate.Article{}, s.getErr
	}
	return generate.Article{ID: 1, Keyword: "x", Status: generate.StatusDraft}, nil
}

func newArticlesRouter(svc *fakeGenerateService) http.Handler {
	healthHandler := handlers.NewHealthHandler(apphealth.NewService(domainhealth.NewService(stubRepo{})))
	server := handlers.NewServer(healthHandler, handlers.NewWordpressSitesHandler(nil), handlers.NewLoginHandler(nil), handlers.NewArticlesHandler(svc))
	return apihttp.NewRouter(config.ServerConfig{
		Host:               "localhost",
		Port:               "0",
		BasePath:           "/",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}, server, nil)
}

func TestArticles_Generate(t *testing.T) {
	router := newArticlesRouter(&fakeGenerateService{})
	rec := doJSON(t, router, http.MethodPost, "/generate", oapigen.GenerateRequest{
		Keyword: "test keyword",
		SiteId:  uuid.New(),
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("generate status = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestArticles_GenerateNoCluster(t *testing.T) {
	router := newArticlesRouter(&fakeGenerateService{genErr: appgen.ErrNoCluster})
	rec := doJSON(t, router, http.MethodPost, "/generate", oapigen.GenerateRequest{
		Keyword: "unknown",
		SiteId:  uuid.New(),
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing-cluster status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestArticles_GenerateZeroSiteID(t *testing.T) {
	router := newArticlesRouter(&fakeGenerateService{})
	rec := doJSON(t, router, http.MethodPost, "/generate", oapigen.GenerateRequest{
		Keyword: "test keyword",
		SiteId:  uuid.Nil,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("zero site_id status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestArticles_List(t *testing.T) {
	router := newArticlesRouter(&fakeGenerateService{arts: []generate.Article{{ID: 1, Keyword: "a", Status: generate.StatusDraft}}})
	rec := doJSON(t, router, http.MethodGet, "/articles", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestArticles_GetMissing(t *testing.T) {
	router := newArticlesRouter(&fakeGenerateService{getErr: appgen.ErrArticleNotFound})
	rec := doJSON(t, router, http.MethodGet, "/articles/99", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestArticles_PublishMissing(t *testing.T) {
	router := newArticlesRouter(&fakeGenerateService{pubErr: appgen.ErrArticleNotFound})
	rec := doJSON(t, router, http.MethodPost, "/articles/99/publish", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("publish missing status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}
