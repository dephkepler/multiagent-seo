package handlers_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	apparticles "multiagent-seo/internal/application/articles"
	apphealth "multiagent-seo/internal/application/health"
	"multiagent-seo/internal/domain/articles"
	domainhealth "multiagent-seo/internal/domain/health"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/config"
)

type fakeGenerateService struct {
	generated articles.Article
	genErr    error
	arts      []articles.Article
	getErr    error
	pubErr    error
}

func (s *fakeGenerateService) Generate(_ context.Context, req apparticles.GenerateRequest) (apparticles.GenerateResult, error) {
	if s.genErr != nil {
		return apparticles.GenerateResult{}, s.genErr
	}
	return apparticles.GenerateResult{
		Article:        articles.Article{ID: 1, Keyword: req.Keyword, SiteID: req.SiteID, Status: articles.StatusGenerating},
		TargetKeywords: []string{req.Keyword},
	}, nil
}

func (s *fakeGenerateService) Publish(context.Context, int64) (articles.Article, error) {
	if s.pubErr != nil {
		return articles.Article{}, s.pubErr
	}
	return articles.Article{ID: 1, Status: articles.StatusPublished}, nil
}

func (s *fakeGenerateService) List(context.Context, int, int) ([]articles.Article, int, error) {
	return s.arts, len(s.arts), nil
}

func (s *fakeGenerateService) Get(context.Context, int64) (articles.Article, error) {
	if s.getErr != nil {
		return articles.Article{}, s.getErr
	}
	return articles.Article{ID: 1, Keyword: "x", Status: articles.StatusDraft}, nil
}
func (s *fakeGenerateService) RateArticle(context.Context, int64, *bool) error { return nil }
func (s *fakeGenerateService) GenerateBatch(context.Context, int, apparticles.GenerateRequest) ([]int64, error) {
	return nil, nil
}

func newArticlesRouter(svc *fakeGenerateService) http.Handler {
	healthHandler := handlers.NewHealthHandler(apphealth.NewService(domainhealth.NewService(stubRepo{})))
	server := handlers.NewServer(healthHandler, handlers.NewWordpressSitesHandler(nil), handlers.NewLoginHandler(nil), handlers.NewArticlesHandler(svc), handlers.NewLinkbuildingHandler(nil), handlers.NewApiTokensHandler(nil), handlers.NewEmailScrapeHandler(nil), handlers.NewLeadStatsHandler(nil))
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
	router := newArticlesRouter(&fakeGenerateService{genErr: apparticles.ErrNoCluster})
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
	router := newArticlesRouter(&fakeGenerateService{arts: []articles.Article{{ID: 1, Keyword: "a", Status: articles.StatusDraft}}})
	rec := doJSON(t, router, http.MethodGet, "/articles", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestArticles_GetMissing(t *testing.T) {
	router := newArticlesRouter(&fakeGenerateService{getErr: apparticles.ErrArticleNotFound})
	rec := doJSON(t, router, http.MethodGet, "/articles/99", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestArticles_PublishMissing(t *testing.T) {
	router := newArticlesRouter(&fakeGenerateService{pubErr: apparticles.ErrArticleNotFound})
	rec := doJSON(t, router, http.MethodPost, "/articles/99/publish", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("publish missing status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}
