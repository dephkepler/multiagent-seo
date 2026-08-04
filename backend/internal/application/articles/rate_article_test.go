package articles_test

import (
	"context"
	"testing"

	apparticles "multiagent-seo/internal/application/articles"
	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/testsupport/fakes"
	"multiagent-seo/pkg/jobrunner"
)

func TestRateArticle(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{5: {ID: 5}}}
	ps := &fakes.PromptStore{}
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
	if ps.LastReward == nil || ps.LastReward.Human == nil {
		t.Fatal("expected an outcome reward update with a human value")
	}
	if *ps.LastReward.Human != 1.0 {
		t.Errorf("like → human = %.2f, want 1.0", *ps.LastReward.Human)
	}
	if ps.LastReward.Stage != articles.PromptStageWriter {
		t.Errorf("stage = %q, want writer", ps.LastReward.Stage)
	}
	if ps.LastReward.Weight != 0.65 {
		t.Errorf("weight = %.2f, want 0.65", ps.LastReward.Weight)
	}
}

func TestRateArticleDislike(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{7: {ID: 7}}}
	ps := &fakes.PromptStore{}
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
	if ps.LastReward == nil || ps.LastReward.Human == nil || *ps.LastReward.Human != 0.0 {
		t.Errorf("dislike → human should be 0.0, got %+v", ps.LastReward)
	}
}

func TestRateArticleClear(t *testing.T) {
	yes := true
	repo := &fakeRepo{arts: map[int64]*articles.Article{9: {ID: 9, HumanRating: &yes}}}
	ps := &fakes.PromptStore{}
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
	if ps.LastReward == nil || ps.LastReward.Human != nil {
		t.Errorf("clear → human should be nil (revert to ai-only), got %+v", ps.LastReward)
	}
}
