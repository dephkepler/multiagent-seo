//go:build integration

package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	apparticles "multiagent-seo/internal/application/articles"
	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/infrastructure/checker"
	"multiagent-seo/internal/infrastructure/dataforseo"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/pexels"
	"multiagent-seo/internal/infrastructure/sheets"
	"multiagent-seo/internal/infrastructure/wordpress"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/internal/testsupport"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/jobrunner"
)

const itArtKnownKeyword = "android game development services"

// The service resolves a site's keyword source through a provider now; this
// hands every site the same mock sheet.
type itArtFakeTopics struct {
	src articles.TopicSource
}

func (f itArtFakeTopics) ForSite(context.Context, uuid.UUID, string) (articles.TopicSource, error) {
	return f.src, nil
}

type itArtFakeLLM struct{}

func (itArtFakeLLM) Complete(context.Context, string, int) (string, articles.Usage, error) {
	return "Generated article body. It has several sentences. Each one is short and clear.", articles.Usage{}, nil
}

type itArtFakeFactory struct{}

func (itArtFakeFactory) ForModel(string, string) (articles.LLMClient, error) {
	return itArtFakeLLM{}, nil
}

func itArtBuild(t *testing.T) http.Handler {
	t.Helper()
	pool := testsupport.NewTestDB(t, baseConnStr)

	articleRepo := postgres.NewArticleRepository(pool)
	serp := dataforseo.NewMock()
	topics := sheets.NewMock()
	check, err := checker.New(checker.Config{Provider: "mock", AIThreshold: 0.9}, nil)
	if err != nil {
		t.Fatalf("checker.New: %v", err)
	}
	images := pexels.New("", nil)
	publisher := wordpress.NewProvider(postgres.NewWordpressSiteRepository(pool, "k"), nil, 10*time.Second)
	prompts := postgres.NewPromptRepository(pool)
	runner := jobrunner.NewSyncRunner()

	svc := apparticles.NewService(
		articleRepo, itArtFakeFactory{}, serp, itArtFakeTopics{src: topics}, check, images, publisher, prompts, runner,
		apparticles.Defaults{
			MinWords: 300, MaxWords: 600, Language: "en",
			Provider: "groq", Model: "m", AIThreshold: 0.9, MaxCycles: 1, SERPLimit: 5,
		},
		nil,
	)

	server := handlers.NewServer(nil, nil, nil, handlers.NewArticlesHandler(svc), handlers.NewLinkbuildingHandler(nil), handlers.NewApiTokensHandler(nil), handlers.NewEmailScrapeHandler(nil), handlers.NewLeadStatsHandler(nil), handlers.NewClientSegmentsHandler(nil), handlers.NewClientDetailHandler(nil), handlers.NewVaultHandler(nil), handlers.NewFinanceHandler(nil))
	return apihttp.NewRouter(config.ServerConfig{
		BasePath:           "/",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}, server, nil)
}

func itArtGenerate(t *testing.T, router http.Handler, keyword string, siteID uuid.UUID) oapigen.GenerateAccepted {
	t.Helper()
	rec := doJSON(t, router, http.MethodPost, "/generate", oapigen.GenerateRequest{
		Keyword: keyword,
		SiteId:  siteID,
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /generate status = %d, want 202 (body=%s)", rec.Code, rec.Body.String())
	}
	var accepted oapigen.GenerateAccepted
	if err := json.NewDecoder(rec.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	return accepted
}

func TestItArt_GenerateAndPersist(t *testing.T) {
	router := itArtBuild(t)
	siteID := uuid.New()

	accepted := itArtGenerate(t, router, itArtKnownKeyword, siteID)
	if accepted.Id == 0 {
		t.Fatalf("accepted.Id = 0, want non-zero")
	}
	if len(accepted.TargetKeywords) == 0 {
		t.Fatalf("target_keywords empty, want populated cluster")
	}

	rec := doJSON(t, router, http.MethodGet, articlePath(accepted.Id), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /articles/{id} status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got oapigen.Article
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode article: %v", err)
	}
	if got.Keyword != itArtKnownKeyword {
		t.Errorf("keyword = %q, want %q", got.Keyword, itArtKnownKeyword)
	}
	if !articles.Status(got.Status).IsTerminal() {
		t.Errorf("status = %q, want terminal (failed/published)", got.Status)
	}
	if got.Status != string(articles.StatusFailed) {
		t.Logf("observed terminal status = %q (expected failed: random site has no WP creds)", got.Status)
	}
	if got.CompetitorData == nil {
		t.Errorf("competitor_data is nil, want SERP mock data persisted")
	}

	rec = doJSON(t, router, http.MethodGet, "/articles", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /articles status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	// GET /articles is paginated now: {items, total}, not a bare array.
	var list oapigen.ArticleList
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if !itArtListHas(list.Items, accepted.Id) {
		t.Errorf("GET /articles missing generated article id=%d", accepted.Id)
	}
}

func TestItArt_PublishNoDraft(t *testing.T) {
	router := itArtBuild(t)
	accepted := itArtGenerate(t, router, itArtKnownKeyword, uuid.New())

	rec := doJSON(t, router, http.MethodPost, articlePath(accepted.Id)+"/publish", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("publish non-draft status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
}

func articlePath(id int64) string {
	return "/articles/" + itArtItoa(id)
}

func itArtItoa(id int64) string {
	const digits = "0123456789"
	if id == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for id > 0 {
		i--
		buf[i] = digits[id%10]
		id /= 10
	}
	return string(buf[i:])
}

func itArtListHas(list []oapigen.Article, id int64) bool {
	for _, a := range list {
		if a.Id == id {
			return true
		}
	}
	return false
}
