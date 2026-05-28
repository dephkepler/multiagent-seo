//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/generate"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

func TestArticleRepository_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	siteID := uuid.New()
	id, err := repo.Create(ctx, generate.CreateArticle{
		Keyword: "best running shoes",
		SiteID:  siteID,
		Site:    "example.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
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
	if got.Status != generate.StatusGenerating {
		t.Errorf("Status = %q, want %q", got.Status, generate.StatusGenerating)
	}
}

func TestArticleRepository_GetNotFound(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	_, err := repo.Get(ctx, 999999)
	if !errors.Is(err, generate.ErrNotFound) {
		t.Fatalf("Get missing id err = %v, want ErrNotFound", err)
	}
}

func TestArticleRepository_List(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	first, err := repo.Create(ctx, generate.CreateArticle{Keyword: "alpha", SiteID: uuid.New(), Site: "a.com"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := repo.Create(ctx, generate.CreateArticle{Keyword: "beta", SiteID: uuid.New(), Site: "b.com"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}

	seen := map[int64]string{}
	for _, a := range list {
		seen[a.ID] = a.Keyword
	}
	if seen[first] != "alpha" {
		t.Errorf("list missing first article, got %v", seen)
	}
	if seen[second] != "beta" {
		t.Errorf("list missing second article, got %v", seen)
	}
}

func TestArticleRepository_UpdateDraft(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	id, err := repo.Create(ctx, generate.CreateArticle{Keyword: "draft me", SiteID: uuid.New(), Site: "d.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const wpPostID = int64(4242)
	const editURL = "https://d.com/wp-admin/post.php?post=4242&action=edit"
	if err := repo.UpdateDraft(ctx, id, wpPostID, editURL); err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != generate.StatusDraft {
		t.Errorf("Status = %q, want %q", got.Status, generate.StatusDraft)
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

	id, err := repo.Create(ctx, generate.CreateArticle{Keyword: "publish me", SiteID: uuid.New(), Site: "p.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const postURL = "https://p.com/publish-me"
	if err := repo.MarkPublished(ctx, id, postURL); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != generate.StatusPublished {
		t.Errorf("Status = %q, want %q", got.Status, generate.StatusPublished)
	}
	if got.WPPostURL != postURL {
		t.Errorf("WPPostURL = %q, want %q", got.WPPostURL, postURL)
	}
}

func TestArticleRepository_MarkFailed(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	id, err := repo.Create(ctx, generate.CreateArticle{Keyword: "fail me", SiteID: uuid.New(), Site: "f.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.MarkFailed(ctx, id); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != generate.StatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, generate.StatusFailed)
	}
}

func TestArticleRepository_SaveCompetitorData(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewArticleRepository(testsupport.NewTestDB(t, baseConnStr))

	id, err := repo.Create(ctx, generate.CreateArticle{Keyword: "serp", SiteID: uuid.New(), Site: "s.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	data := generate.CompetitorData{
		Keyword:  "serp",
		SerpDate: "2026-05-28",
		Results:  []generate.SERPItem{{Rank: 1, URL: "https://rival.com", Title: "Rival"}},
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

	var roundTrip generate.CompetitorData
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

	id, err := repo.Create(ctx, generate.CreateArticle{Keyword: "check", SiteID: uuid.New(), Site: "c.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	result := generate.CheckResult{AIScore: 0.12, PlagiarismScore: 0.03, Original: true, Provider: "test"}
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

	var roundTrip generate.CheckResult
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

	id, err := repo.Create(ctx, generate.CreateArticle{Keyword: "images", SiteID: uuid.New(), Site: "i.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

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
