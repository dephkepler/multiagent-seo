package promptevolution_test

import (
	"context"
	"errors"
	"testing"

	"multiagent-seo/internal/application/promptevolution"
	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/testsupport/fakes"
)

var errBoom = errors.New("boom")

func champStat(id int64) articles.PromptVariantStat {
	return articles.PromptVariantStat{
		Variant: articles.PromptVariant{ID: id, Stage: articles.PromptStageWriter, Status: articles.VariantChampion, Body: "champion prompt body"},
		Samples: 50,
	}
}

func candStat(id int64) articles.PromptVariantStat {
	return articles.PromptVariantStat{Variant: articles.PromptVariant{ID: id, Status: articles.VariantCandidate}, Samples: 5}
}

type okLLM struct{}

func (okLLM) Complete(context.Context, string, int) (string, articles.Usage, error) {
	return "generated body", articles.Usage{}, nil
}

type okLLMFactory struct{}

func (okLLMFactory) ForModel(string, string) (articles.LLMClient, error) { return okLLM{}, nil }

type emptyLLM struct{}

func (emptyLLM) Complete(context.Context, string, int) (string, articles.Usage, error) {
	return "   ", articles.Usage{}, nil
}

type emptyLLMFactory struct{}

func (emptyLLMFactory) ForModel(string, string) (articles.LLMClient, error) { return emptyLLM{}, nil }

type errLLMFactory struct{}

func (errLLMFactory) ForModel(string, string) (articles.LLMClient, error) { return nil, errBoom }

func TestGenerateCandidate_HappyPath(t *testing.T) {
	ps := &fakes.PromptStore{
		Stats:    []articles.PromptVariantStat{champStat(7)},
		Failures: []articles.PromptFailure{{ArticleID: 1, AIScore: 0.9, Flagged: []string{"In conclusion, it matters."}}},
	}
	promptevolution.NewService(ps, okLLMFactory{}, nil).GenerateCandidate(context.Background())

	if len(ps.Inserted) != 1 {
		t.Fatalf("want 1 inserted candidate, got %d", len(ps.Inserted))
	}
	got := ps.Inserted[0]
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
		ps      *fakes.PromptStore
		factory articles.LLMFactory
	}{
		"stats error":       {&fakes.PromptStore{StatsErr: errBoom}, okLLMFactory{}},
		"no champion":       {&fakes.PromptStore{Stats: []articles.PromptVariantStat{candStat(1)}}, okLLMFactory{}},
		"slots full":        {&fakes.PromptStore{Stats: []articles.PromptVariantStat{champStat(7), candStat(1), candStat(2)}}, okLLMFactory{}},
		"no failures":       {&fakes.PromptStore{Stats: []articles.PromptVariantStat{champStat(7)}}, okLLMFactory{}},
		"failures error":    {&fakes.PromptStore{Stats: []articles.PromptVariantStat{champStat(7)}, FailErr: errBoom}, okLLMFactory{}},
		"llm factory error": {&fakes.PromptStore{Stats: []articles.PromptVariantStat{champStat(7)}, Failures: []articles.PromptFailure{{ArticleID: 1, AIScore: 0.9}}}, errLLMFactory{}},
		"llm empty output":  {&fakes.PromptStore{Stats: []articles.PromptVariantStat{champStat(7)}, Failures: []articles.PromptFailure{{ArticleID: 1, AIScore: 0.9}}}, emptyLLMFactory{}},
		"insert error":      {&fakes.PromptStore{Stats: []articles.PromptVariantStat{champStat(7)}, Failures: []articles.PromptFailure{{ArticleID: 1, AIScore: 0.9}}, InsertErr: errBoom}, okLLMFactory{}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			promptevolution.NewService(c.ps, c.factory, nil).GenerateCandidate(context.Background())
			if len(c.ps.Inserted) != 0 {
				t.Errorf("expected no inserted candidate for %q, got %d", name, len(c.ps.Inserted))
			}
		})
	}
}

func TestGenerateCandidate_NilStoreNoPanic(t *testing.T) {
	promptevolution.NewService(nil, okLLMFactory{}, nil).GenerateCandidate(context.Background())
}
