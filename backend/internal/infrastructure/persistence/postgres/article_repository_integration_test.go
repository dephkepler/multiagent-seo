//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

func TestArticleRepository_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	siteID := uuid.New()
	created, err := repo.Create(ctx, articles.CreateArticle{
		Keyword: "best running shoes",
		SiteID:  siteID,
		Site:    "example.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID
	if id == 0 {
		t.Fatalf("Create returned zero id")
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Keyword != "best running shoes" {
		t.Errorf("Keyword = %q, want %q", got.Keyword, "best running shoes")
	}
	if got.SiteID != siteID {
		t.Errorf("SiteID = %v, want %v", got.SiteID, siteID)
	}
	if got.Status != articles.StatusGenerating {
		t.Errorf("Status = %q, want %q", got.Status, articles.StatusGenerating)
	}
}

func TestArticleRepository_GetNotFound(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	_, err := repo.Get(ctx, 999999)
	if !errors.Is(err, articles.ErrNotFound) {
		t.Fatalf("Get missing id err = %v, want ErrNotFound", err)
	}
}

func TestArticleRepository_List(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	first, err := repo.Create(ctx, articles.CreateArticle{Keyword: "alpha", SiteID: uuid.New(), Site: "a.com"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := repo.Create(ctx, articles.CreateArticle{Keyword: "beta", SiteID: uuid.New(), Site: "b.com"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	list, total, err := repo.List(ctx, 25, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Fatalf("List total = %d, want 2", total)
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}

	seen := map[int64]string{}
	for _, a := range list {
		seen[a.ID] = a.Keyword
	}
	if seen[first.ID] != "alpha" {
		t.Errorf("list missing first article, got %v", seen)
	}
	if seen[second.ID] != "beta" {
		t.Errorf("list missing second article, got %v", seen)
	}
}

func TestArticleRepository_UpdateDraft(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "draft me", SiteID: uuid.New(), Site: "d.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	const wpPostID = int64(4242)
	const editURL = "https://d.com/wp-admin/post.php?post=4242&action=edit"
	if err := repo.UpdateDraft(ctx, id, wpPostID, editURL); err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != articles.StatusDraft {
		t.Errorf("Status = %q, want %q", got.Status, articles.StatusDraft)
	}
	if got.WPPostID != wpPostID {
		t.Errorf("WPPostID = %d, want %d", got.WPPostID, wpPostID)
	}
	if got.WPEditURL != editURL {
		t.Errorf("WPEditURL = %q, want %q", got.WPEditURL, editURL)
	}
}

func TestArticleRepository_MarkPublished(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "publish me", SiteID: uuid.New(), Site: "p.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	const postURL = "https://p.com/publish-me"
	if err := repo.MarkPublished(ctx, id, postURL); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != articles.StatusPublished {
		t.Errorf("Status = %q, want %q", got.Status, articles.StatusPublished)
	}
	if got.WPPostURL != postURL {
		t.Errorf("WPPostURL = %q, want %q", got.WPPostURL, postURL)
	}
}

func TestArticleRepository_MarkFailed(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "fail me", SiteID: uuid.New(), Site: "f.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	if err := repo.MarkFailed(ctx, id); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != articles.StatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, articles.StatusFailed)
	}
}

func TestArticleRepository_SaveCompetitorData(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "serp", SiteID: uuid.New(), Site: "s.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	data := articles.CompetitorData{
		Keyword:  "serp",
		SerpDate: "2026-05-28",
		Results:  []articles.SERPItem{{Rank: 1, URL: "https://rival.com", Title: "Rival"}},
	}
	if err := repo.SaveCompetitorData(ctx, id, data); err != nil {
		t.Fatalf("SaveCompetitorData: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.CompetitorData) == 0 {
		t.Fatalf("CompetitorData empty after save")
	}

	var roundTrip articles.CompetitorData
	if err := json.Unmarshal(got.CompetitorData, &roundTrip); err != nil {
		t.Fatalf("unmarshal CompetitorData: %v", err)
	}
	if roundTrip.Keyword != "serp" || len(roundTrip.Results) != 1 {
		t.Errorf("CompetitorData round-trip mismatch: %+v", roundTrip)
	}
}

func TestArticleRepository_SaveCheckResult(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "check", SiteID: uuid.New(), Site: "c.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	result := articles.CheckResult{AIScore: 0.12, PlagiarismScore: 0.03, Original: true, Provider: "test"}
	if err := repo.SaveCheckResult(ctx, id, result); err != nil {
		t.Fatalf("SaveCheckResult: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.CheckResult) == 0 {
		t.Fatalf("CheckResult empty after save")
	}

	var roundTrip articles.CheckResult
	if err := json.Unmarshal(got.CheckResult, &roundTrip); err != nil {
		t.Fatalf("unmarshal CheckResult: %v", err)
	}
	if !roundTrip.Original || roundTrip.Provider != "test" {
		t.Errorf("CheckResult round-trip mismatch: %+v", roundTrip)
	}
}

func TestArticleRepository_SaveImageStats(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "images", SiteID: uuid.New(), Site: "i.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	if err := repo.SaveImageStats(ctx, id, 5, 3, 2); err != nil {
		t.Fatalf("SaveImageStats: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ImagesRequested != 5 || got.ImagesResolved != 3 || got.ImagesSkipped != 2 {
		t.Errorf("image stats = (%d,%d,%d), want (5,3,2)",
			got.ImagesRequested, got.ImagesResolved, got.ImagesSkipped)
	}
}

func TestArticleRepository_GeneratedKeywords_SiteScopedAndSkipsFailed(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	siteA := uuid.New()
	siteB := uuid.New()

	if _, err := repo.Create(ctx, articles.CreateArticle{Keyword: "alpha", SiteID: siteA, Site: "a.com"}); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	if _, err := repo.Create(ctx, articles.CreateArticle{Keyword: "beta", SiteID: siteA, Site: "a.com"}); err != nil {
		t.Fatalf("Create beta: %v", err)
	}
	if _, err := repo.Create(ctx, articles.CreateArticle{Keyword: "gamma", SiteID: siteB, Site: "b.com"}); err != nil {
		t.Fatalf("Create gamma: %v", err)
	}
	delta, err := repo.Create(ctx, articles.CreateArticle{Keyword: "delta", SiteID: siteA, Site: "a.com"})
	if err != nil {
		t.Fatalf("Create delta: %v", err)
	}
	if err := repo.MarkFailed(ctx, delta.ID); err != nil {
		t.Fatalf("MarkFailed delta: %v", err)
	}

	gotA, err := repo.GeneratedKeywords(ctx, siteA)
	if err != nil {
		t.Fatalf("GeneratedKeywords siteA: %v", err)
	}
	if got, want := toSet(gotA), map[string]bool{"alpha": true, "beta": true}; !equalSet(got, want) {
		t.Errorf("siteA keywords = %v, want alpha+beta only (no other-site gamma, no failed delta)", gotA)
	}

	gotB, err := repo.GeneratedKeywords(ctx, siteB)
	if err != nil {
		t.Fatalf("GeneratedKeywords siteB: %v", err)
	}
	if got, want := toSet(gotB), map[string]bool{"gamma": true}; !equalSet(got, want) {
		t.Errorf("siteB keywords = %v, want gamma only", gotB)
	}
}

func TestArticleRepository_GeneratedKeywords_EmptyForUnknownSite(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	siteA := uuid.New()
	if _, err := repo.Create(ctx, articles.CreateArticle{Keyword: "alpha", SiteID: siteA, Site: "a.com"}); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}

	kws, err := repo.GeneratedKeywords(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GeneratedKeywords unknown site: %v", err)
	}
	if kws == nil {
		t.Fatalf("GeneratedKeywords returned nil slice, want non-nil empty (make([]string, 0) contract)")
	}
	if len(kws) != 0 {
		t.Errorf("GeneratedKeywords len = %d, want 0", len(kws))
	}
}

func TestArticleRepository_Get_NoWriterOutcome_NilFields(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "join-nil", SiteID: uuid.New(), Site: "a.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Keyword != "join-nil" {
		t.Errorf("Keyword = %q, want %q", got.Keyword, "join-nil")
	}
	if got.AIScore != nil {
		t.Errorf("AIScore = %v, want nil", *got.AIScore)
	}
	if got.Reward != nil {
		t.Errorf("Reward = %v, want nil", *got.Reward)
	}
	if got.QualityOK != nil {
		t.Errorf("QualityOK = %v, want nil", *got.QualityOK)
	}
	if got.HumanizeCycles != nil {
		t.Errorf("HumanizeCycles = %v, want nil", *got.HumanizeCycles)
	}
	if got.Tokens != nil {
		t.Errorf("Tokens = %v, want nil", *got.Tokens)
	}
}

func TestArticleRepository_Get_WriterOutcomePopulatesInfoPanelFields(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewArticleRepository(pool)
	prompts := postgres.NewPromptRepository(pool)

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "join-full", SiteID: uuid.New(), Site: "a.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	if err := prompts.SaveOutcome(ctx, articles.PromptOutcome{
		ArticleID:      id,
		Stage:          articles.PromptStageWriter,
		AIScore:        ptr(0.25),
		Reward:         ptr(0.75),
		QualityOK:      true,
		HumanizeCycles: 2,
		Tokens:         1500,
	}); err != nil {
		t.Fatalf("SaveOutcome: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AIScore == nil || !floatNear(*got.AIScore, 0.25) {
		t.Errorf("AIScore = %v, want ~0.25", got.AIScore)
	}
	if got.Reward == nil || !floatNear(*got.Reward, 0.75) {
		t.Errorf("Reward = %v, want ~0.75", got.Reward)
	}
	if got.QualityOK == nil || *got.QualityOK != true {
		t.Errorf("QualityOK = %v, want true", got.QualityOK)
	}
	if got.HumanizeCycles == nil || *got.HumanizeCycles != 2 {
		t.Errorf("HumanizeCycles = %v, want 2", got.HumanizeCycles)
	}
	if got.Tokens == nil || *got.Tokens != 1500 {
		t.Errorf("Tokens = %v, want 1500", got.Tokens)
	}
}

func TestArticleRepository_Get_IgnoresNonWriterOutcome(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewArticleRepository(pool)
	prompts := postgres.NewPromptRepository(pool)

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "join-editor", SiteID: uuid.New(), Site: "a.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	if err := prompts.SaveOutcome(ctx, articles.PromptOutcome{
		ArticleID:      id,
		Stage:          articles.PromptStageEditor,
		AIScore:        ptr(0.9),
		Reward:         ptr(0.1),
		QualityOK:      true,
		HumanizeCycles: 7,
		Tokens:         9000,
	}); err != nil {
		t.Fatalf("SaveOutcome: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AIScore != nil || got.Reward != nil || got.QualityOK != nil ||
		got.HumanizeCycles != nil || got.Tokens != nil {
		t.Errorf("non-writer outcome leaked into Get: ai=%v reward=%v quality=%v cycles=%v tokens=%v",
			got.AIScore, got.Reward, got.QualityOK, got.HumanizeCycles, got.Tokens)
	}
}

func TestArticleRepository_Get_WriterOutcomeNullScalars(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewArticleRepository(pool)
	prompts := postgres.NewPromptRepository(pool)

	created, err := repo.Create(ctx, articles.CreateArticle{Keyword: "join-nullable", SiteID: uuid.New(), Site: "a.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := created.ID

	if err := prompts.SaveOutcome(ctx, articles.PromptOutcome{
		ArticleID:      id,
		Stage:          articles.PromptStageWriter,
		AIScore:        nil,
		Reward:         nil,
		QualityOK:      false,
		HumanizeCycles: 0,
		Tokens:         0,
	}); err != nil {
		t.Fatalf("SaveOutcome: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AIScore != nil {
		t.Errorf("AIScore = %v, want nil (nullable REAL)", *got.AIScore)
	}
	if got.Reward != nil {
		t.Errorf("Reward = %v, want nil (nullable REAL)", *got.Reward)
	}
	if got.QualityOK == nil || *got.QualityOK != false {
		t.Errorf("QualityOK = %v, want non-nil false", got.QualityOK)
	}
	if got.HumanizeCycles == nil || *got.HumanizeCycles != 0 {
		t.Errorf("HumanizeCycles = %v, want non-nil 0", got.HumanizeCycles)
	}
	if got.Tokens == nil || *got.Tokens != 0 {
		t.Errorf("Tokens = %v, want non-nil 0", got.Tokens)
	}
}

func ptr[T any](v T) *T { return &v }

func floatNear(got, want float64) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d < 1e-4
}

func toSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}

func equalSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
