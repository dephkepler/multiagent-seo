package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/articles"
)

func TestToArticle_AllNullablePopulated(t *testing.T) {
	published := time.Now()
	aiScore := 0.42
	reward := 0.91
	qualityOK := true
	cycles := 4
	tokens := 12345
	rating := true
	rp := json.RawMessage(`{"k":"v"}`)

	out := toArticle(articles.Article{
		ID:             7,
		Keyword:        "kw",
		Status:         articles.StatusDraft,
		PublishedAt:    &published,
		RequestParams:  rp,
		HumanRating:    &rating,
		AIScore:        &aiScore,
		Reward:         &reward,
		QualityOK:      &qualityOK,
		HumanizeCycles: &cycles,
		Tokens:         &tokens,
	})

	if out.AiScore == nil || *out.AiScore != float32(0.42) {
		t.Fatalf("AiScore = %v, want float32 0.42", out.AiScore)
	}
	if out.Reward == nil || *out.Reward != float32(0.91) {
		t.Fatalf("Reward = %v, want float32 0.91", out.Reward)
	}
	if out.QualityOk == nil || *out.QualityOk != true {
		t.Fatalf("QualityOk = %v, want true", out.QualityOk)
	}
	if out.HumanizeCycles == nil || *out.HumanizeCycles != 4 {
		t.Fatalf("HumanizeCycles = %v, want 4", out.HumanizeCycles)
	}
	if out.Tokens == nil || *out.Tokens != 12345 {
		t.Fatalf("Tokens = %v, want 12345", out.Tokens)
	}
	if out.PublishedAt == nil || !out.PublishedAt.Equal(published) {
		t.Fatalf("PublishedAt = %v, want %v", out.PublishedAt, published)
	}
	if out.HumanRating == nil || *out.HumanRating != true {
		t.Fatalf("HumanRating = %v, want true", out.HumanRating)
	}
	if out.RequestParams == nil || string(*out.RequestParams) != `{"k":"v"}` {
		t.Fatalf("RequestParams = %v, want raw json", out.RequestParams)
	}
}

func TestToArticle_AllNullableNil(t *testing.T) {
	out := toArticle(articles.Article{
		ID:      1,
		Keyword: "kw",
		Status:  articles.StatusDraft,
		SiteID:  uuid.Nil,
	})

	if out.AiScore != nil {
		t.Fatalf("AiScore = %v, want nil", out.AiScore)
	}
	if out.Reward != nil {
		t.Fatalf("Reward = %v, want nil", out.Reward)
	}
	if out.QualityOk != nil {
		t.Fatalf("QualityOk = %v, want nil", out.QualityOk)
	}
	if out.HumanizeCycles != nil {
		t.Fatalf("HumanizeCycles = %v, want nil", out.HumanizeCycles)
	}
	if out.Tokens != nil {
		t.Fatalf("Tokens = %v, want nil", out.Tokens)
	}
	if out.PublishedAt != nil {
		t.Fatalf("PublishedAt = %v, want nil", out.PublishedAt)
	}
	if out.HumanRating != nil {
		t.Fatalf("HumanRating = %v, want nil", out.HumanRating)
	}
	if out.RequestParams != nil {
		t.Fatalf("RequestParams = %v, want nil", out.RequestParams)
	}

	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"ai_score", "reward", "quality_ok", "humanize_cycles", "tokens", "published_at", "human_rating", "request_params"} {
		if containsKey(b, key) {
			t.Fatalf("json contains %q, want absent (body=%s)", key, b)
		}
	}
}

func TestToArticle_ZeroButPresentScalars(t *testing.T) {
	aiScore := 0.0
	reward := 0.0
	qualityOK := false
	cycles := 0
	tokens := 0

	out := toArticle(articles.Article{
		ID:             2,
		Keyword:        "kw",
		Status:         articles.StatusDraft,
		AIScore:        &aiScore,
		Reward:         &reward,
		QualityOK:      &qualityOK,
		HumanizeCycles: &cycles,
		Tokens:         &tokens,
	})

	if out.AiScore == nil || *out.AiScore != 0 {
		t.Fatalf("AiScore = %v, want pointer to 0", out.AiScore)
	}
	if out.Reward == nil || *out.Reward != 0 {
		t.Fatalf("Reward = %v, want pointer to 0", out.Reward)
	}
	if out.QualityOk == nil || *out.QualityOk != false {
		t.Fatalf("QualityOk = %v, want pointer to false", out.QualityOk)
	}
	if out.HumanizeCycles == nil || *out.HumanizeCycles != 0 {
		t.Fatalf("HumanizeCycles = %v, want pointer to 0", out.HumanizeCycles)
	}
	if out.Tokens == nil || *out.Tokens != 0 {
		t.Fatalf("Tokens = %v, want pointer to 0", out.Tokens)
	}

	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"ai_score", "reward", "quality_ok", "humanize_cycles", "tokens"} {
		if !containsKey(b, key) {
			t.Fatalf("json missing %q, want present (body=%s)", key, b)
		}
	}
}

func containsKey(b []byte, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}
