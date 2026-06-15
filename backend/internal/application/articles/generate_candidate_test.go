package articles_test

import (
	"context"
	"errors"
	"testing"
	"time"

	apparticles "multiagent-seo/internal/application/articles"
	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/pkg/jobrunner"
)

var errBoom = errors.New("boom")

type fakePromptStore struct {
	stats     []articles.PromptVariantStat
	statsErr  error
	failures  []articles.PromptFailure
	failErr   error
	insertErr  error
	inserted   []articles.PromptVariant
	statuses   map[int64]articles.VariantStatus
	lastReward *rewardUpdate
}

func (f *fakePromptStore) ActiveVariants(context.Context, string) ([]articles.PromptVariant, error) {
	return nil, nil
}
func (f *fakePromptStore) InsertVariant(_ context.Context, v articles.PromptVariant) (int64, error) {
	if f.insertErr != nil {
		return 0, f.insertErr
	}
	f.inserted = append(f.inserted, v)
	return int64(len(f.inserted)), nil
}
func (f *fakePromptStore) SetVariantStatus(_ context.Context, id int64, st articles.VariantStatus) error {
	if f.statuses == nil {
		f.statuses = map[int64]articles.VariantStatus{}
	}
	f.statuses[id] = st
	return nil
}
func (f *fakePromptStore) SaveOutcome(context.Context, articles.PromptOutcome) error { return nil }
func (f *fakePromptStore) SelectionStats(context.Context, string, time.Time) ([]articles.PromptVariantStat, error) {
	return f.stats, f.statsErr
}
func (f *fakePromptStore) WorstOutcomes(context.Context, int64, time.Time, int) ([]articles.PromptFailure, error) {
	return f.failures, f.failErr
}
type rewardUpdate struct {
	articleID int64
	stage     string
	human     *float64
	weight    float64
}

func (f *fakePromptStore) UpdateOutcomeReward(_ context.Context, articleID int64, stage string, human *float64, weight float64) error {
	f.lastReward = &rewardUpdate{articleID, stage, human, weight}
	return nil
}

func evolveServiceWith(ps articles.PromptStore, f articles.LLMFactory) *apparticles.Service {
	return apparticles.NewService(
		&fakeRepo{arts: map[int64]*articles.Article{}},
		f, fakeSERP{}, fakeTopics{}, fakeChecker{}, fakeImages{}, fakePubProvider{},
		ps,
		jobrunner.NewSyncRunner(),
		apparticles.Defaults{MinWords: 500, MaxWords: 1000, Language: "en", Provider: "groq", Model: "m", AIThreshold: 0.8, MaxCycles: 2, SERPLimit: 5},
		nil,
	)
}

func champStat(id int64) articles.PromptVariantStat {
	return articles.PromptVariantStat{
		Variant: articles.PromptVariant{ID: id, Stage: articles.PromptStageWriter, Status: articles.VariantChampion, Body: "champion prompt body"},
		Samples: 50,
	}
}

func candStat(id int64) articles.PromptVariantStat {
	return articles.PromptVariantStat{Variant: articles.PromptVariant{ID: id, Status: articles.VariantCandidate}, Samples: 5}
}

type emptyLLM struct{}

func (emptyLLM) Complete(context.Context, string, int) (string, articles.Usage, error) {
	return "   ", articles.Usage{}, nil
}

type emptyLLMFactory struct{}

func (emptyLLMFactory) ForModel(string, string) (articles.LLMClient, error) { return emptyLLM{}, nil }

type errLLMFactory struct{}

func (errLLMFactory) ForModel(string, string) (articles.LLMClient, error) { return nil, errBoom }

func TestGenerateCandidate_HappyPath(t *testing.T) {
	ps := &fakePromptStore{
		stats:    []articles.PromptVariantStat{champStat(7)},
		failures: []articles.PromptFailure{{ArticleID: 1, AIScore: 0.9, Flagged: []string{"In conclusion, it matters."}}},
	}
	evolveServiceWith(ps, fakeLLMFactory{}).GenerateCandidate(context.Background())

	if len(ps.inserted) != 1 {
		t.Fatalf("want 1 inserted candidate, got %d", len(ps.inserted))
	}
	got := ps.inserted[0]
	if got.Status != articles.VariantCandidate {
		t.Errorf("status = %v, want candidate", got.Status)
	}
	if got.Origin != articles.OriginGenerated {
		t.Errorf("origin = %v, want generated", got.Origin)
	}
	if got.ParentID == nil || *got.ParentID != 7 {
		t.Errorf("parent = %v, want 7", got.ParentID)
	}
	if got.Body != "generated body" {
		t.Errorf("body = %q, want %q", got.Body, "generated body")
	}
}

func TestGenerateCandidate_Skips(t *testing.T) {
	cases := map[string]struct {
		ps      *fakePromptStore
		factory articles.LLMFactory
	}{
		"nil-like stats error": {&fakePromptStore{statsErr: errBoom}, fakeLLMFactory{}},
		"no champion":          {&fakePromptStore{stats: []articles.PromptVariantStat{candStat(1)}}, fakeLLMFactory{}},
		"slots full":           {&fakePromptStore{stats: []articles.PromptVariantStat{champStat(7), candStat(1), candStat(2)}}, fakeLLMFactory{}},
		"no failures":          {&fakePromptStore{stats: []articles.PromptVariantStat{champStat(7)}}, fakeLLMFactory{}},
		"failures error":       {&fakePromptStore{stats: []articles.PromptVariantStat{champStat(7)}, failErr: errBoom}, fakeLLMFactory{}},
		"llm factory error":    {&fakePromptStore{stats: []articles.PromptVariantStat{champStat(7)}, failures: []articles.PromptFailure{{ArticleID: 1, AIScore: 0.9}}}, errLLMFactory{}},
		"llm empty output":     {&fakePromptStore{stats: []articles.PromptVariantStat{champStat(7)}, failures: []articles.PromptFailure{{ArticleID: 1, AIScore: 0.9}}}, emptyLLMFactory{}},
		"insert error":         {&fakePromptStore{stats: []articles.PromptVariantStat{champStat(7)}, failures: []articles.PromptFailure{{ArticleID: 1, AIScore: 0.9}}, insertErr: errBoom}, fakeLLMFactory{}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			evolveServiceWith(c.ps, c.factory).GenerateCandidate(context.Background())
			if len(c.ps.inserted) != 0 {
				t.Errorf("expected no inserted candidate for %q, got %d", name, len(c.ps.inserted))
			}
		})
	}
}

func TestGenerateCandidate_NilStoreNoPanic(t *testing.T) {
	evolveServiceWith(nil, fakeLLMFactory{}).GenerateCandidate(context.Background())
}
