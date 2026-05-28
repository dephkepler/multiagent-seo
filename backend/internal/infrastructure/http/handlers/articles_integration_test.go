//go:build integration

package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	appgen "multiagent-seo/internal/application/generate"
	"multiagent-seo/internal/domain/generate"
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
)

// itArtKnownKeyword is a topic the sheets mock knows (see sheets/mock.go), so
// the cluster lookup succeeds and the pipeline runs.
const itArtKnownKeyword = "android game development services"

type itArtFakeLLM struct{}

func (itArtFakeLLM) Complete(context.Context, string, int) (string, generate.Usage, error) {
	return "Generated article body. It has several sentences. Each one is short and clear.", generate.Usage{}, nil
}

type itArtFakeFactory struct{}

func (itArtFakeFactory) ForModel(string, string) (generate.LLMClient, error) {
	return itArtFakeLLM{}, nil
}

// itArtBuild wires the real generate Service over a fresh migrated DB and
// returns an in-process chi router plus the underlying article repository.
func itArtBuild(t *testing.T) http.Handler {
	t.Helper()
	pool := testsupport.NewTestDB(t, baseConnStr)

	articleRepo := postgres.NewArticleRepository(pool)
	serp := dataforseo.NewMock()
	topics := sheets.NewMock()
	check, err := checker.New("mock", "", "", 0.9, nil)
	if err != nil {
		t.Fatalf("checker.New: %v", err)
	}
	images := pexels.New("", nil)
	publisher := wordpress.NewProvider(postgres.NewWordpressSiteRepository(pool, "k"), nil)
	runner := appgen.NewSyncRunner()

	svc := appgen.NewService(
		articleRepo, itArtFakeFactory{}, serp, topics, check, images, publisher, runner,
		appgen.Defaults{
			MinWords: 300, MaxWords: 600, Language: "en",
			Provider: "groq", Model: "m", AIThreshold: 0.9, MaxCycles: 1, SERPLimit: 5,
		},
		nil,
	)

	server := handlers.NewServer(nil, nil, nil, handlers.NewArticlesHandler(svc))
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

// TestItArt_GenerateAndPersist drives the full pipeline through the real Service
// and article repository. The SyncRunner runs the pipeline inline, so by the
// time /generate returns the article is already persisted. The site_id is
// random and not stored, so the publisher's ForSite fails and the run ends in a
// terminal "failed" status — but the SERP step ran first, so competitor_data is
// persisted and the article is retrievable.
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
	if !generate.Status(got.Status).IsTerminal() {
		t.Errorf("status = %q, want terminal (failed/published)", got.Status)
	}
	if got.Status != string(generate.StatusFailed) {
		t.Logf("observed terminal status = %q (expected failed: random site has no WP creds)", got.Status)
	}
	if got.CompetitorData == nil {
		t.Errorf("competitor_data is nil, want SERP mock data persisted")
	}

	rec = doJSON(t, router, http.MethodGet, "/articles", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /articles status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var list []oapigen.Article
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if !itArtListHas(list, accepted.Id) {
		t.Errorf("GET /articles missing generated article id=%d", accepted.Id)
	}
}

func TestItArt_GenerateZeroSiteID(t *testing.T) {
	router := itArtBuild(t)
	rec := doJSON(t, router, http.MethodPost, "/generate", oapigen.GenerateRequest{
		Keyword: itArtKnownKeyword,
		SiteId:  uuid.Nil,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("zero site_id status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestItArt_GenerateUnknownKeyword(t *testing.T) {
	router := itArtBuild(t)
	rec := doJSON(t, router, http.MethodPost, "/generate", oapigen.GenerateRequest{
		Keyword: "no such topic in the mock",
		SiteId:  uuid.New(),
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown keyword status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestItArt_GetMissing(t *testing.T) {
	router := itArtBuild(t)
	rec := doJSON(t, router, http.MethodGet, "/articles/999999", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestItArt_PublishMissing(t *testing.T) {
	router := itArtBuild(t)
	rec := doJSON(t, router, http.MethodPost, "/articles/999999/publish", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("publish missing status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestItArt_PublishNoDraft publishes the just-generated article, which ended in
// "failed" (no reachable WP), so it has no draft to promote → 409.
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
