package articles_test

import (
	"context"
	"testing"

	apparticles "multiagent-seo/internal/application/articles"
	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/pkg/jobrunner"
)

func TestRateArticle(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{5: {ID: 5}}}
	ps := &fakePromptStore{}
	svc := apparticles.NewService(
		repo, fakeLLMFactory{}, fakeSERP{}, fakeTopics{}, fakeChecker{}, fakeImages{}, fakePubProvider{},
		ps,
		jobrunner.NewSyncRunner(),
		apparticles.Defaults{HumanWeight: 0.65},
		nil,
	)

	like := true
	if err := svc.RateArticle(context.Background(), 5, &like); err != nil {
		t.Fatalf("RateArticle: %v", err)
	}

	if repo.arts[5].HumanRating == nil || !*repo.arts[5].HumanRating {
		t.Error("expected human_rating=true stored on the article")
	}
	if ps.lastReward == nil || ps.lastReward.human == nil {
		t.Fatal("expected an outcome reward update with a human value")
	}
	if *ps.lastReward.human != 1.0 {
		t.Errorf("like → human = %.2f, want 1.0", *ps.lastReward.human)
	}
	if ps.lastReward.stage != articles.PromptStageWriter {
		t.Errorf("stage = %q, want writer", ps.lastReward.stage)
	}
	if ps.lastReward.weight != 0.65 {
		t.Errorf("weight = %.2f, want 0.65", ps.lastReward.weight)
	}
}

func TestRateArticleDislike(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{7: {ID: 7}}}
	ps := &fakePromptStore{}
	svc := apparticles.NewService(
		repo, fakeLLMFactory{}, fakeSERP{}, fakeTopics{}, fakeChecker{}, fakeImages{}, fakePubProvider{},
		ps, jobrunner.NewSyncRunner(), apparticles.Defaults{HumanWeight: 0.65}, nil,
	)

	dislike := false
	if err := svc.RateArticle(context.Background(), 7, &dislike); err != nil {
		t.Fatalf("RateArticle: %v", err)
	}
	if repo.arts[7].HumanRating == nil || *repo.arts[7].HumanRating {
		t.Error("expected human_rating=false stored")
	}
	if ps.lastReward == nil || ps.lastReward.human == nil || *ps.lastReward.human != 0.0 {
		t.Errorf("dislike → human should be 0.0, got %+v", ps.lastReward)
	}
}

func TestRateArticleClear(t *testing.T) {
	yes := true
	repo := &fakeRepo{arts: map[int64]*articles.Article{9: {ID: 9, HumanRating: &yes}}}
	ps := &fakePromptStore{}
	svc := apparticles.NewService(
		repo, fakeLLMFactory{}, fakeSERP{}, fakeTopics{}, fakeChecker{}, fakeImages{}, fakePubProvider{},
		ps, jobrunner.NewSyncRunner(), apparticles.Defaults{HumanWeight: 0.65}, nil,
	)

	if err := svc.RateArticle(context.Background(), 9, nil); err != nil {
		t.Fatalf("RateArticle clear: %v", err)
	}
	if repo.arts[9].HumanRating != nil {
		t.Error("clearing should reset human_rating to nil")
	}
	if ps.lastReward == nil || ps.lastReward.human != nil {
		t.Errorf("clear → human should be nil (revert to ai-only), got %+v", ps.lastReward)
	}
}
