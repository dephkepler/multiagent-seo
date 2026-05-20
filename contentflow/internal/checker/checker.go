package checker

import (
	"context"
	"math/rand"
	"strings"
)

type Result struct {
	AIScore          float64  `json:"ai_score"`
	PlagiarismScore  float64  `json:"plagiarism_score"`
	Original         bool     `json:"original"`
	Provider         string   `json:"provider"`
	Issues           []string `json:"issues,omitempty"`
	SentencesFlagged []string `json:"sentences_flagged,omitempty"`
	ReportURL        string   `json:"report_url,omitempty"`
}

type Client interface {
	Check(ctx context.Context, content string) (*Result, error)
}

type MockClient struct {
	AIThreshold float64
}

func NewMock(aiThreshold float64) *MockClient {
	if aiThreshold == 0 {
		aiThreshold = 0.8
	}
	return &MockClient{AIThreshold: aiThreshold}
}

func (m *MockClient) Check(_ context.Context, content string) (*Result, error) {
	words := len(strings.Fields(content))
	sentences := strings.Count(content, ".") + strings.Count(content, "!") + strings.Count(content, "?")

	baseAI := 0.55
	if words > 1000 {
		baseAI -= 0.15
	} else if words > 500 {
		baseAI -= 0.08
	}

	if sentences > 0 && words/sentences < 18 {
		baseAI -= 0.10
	}

	aiScore := baseAI + (rand.Float64()*0.16 - 0.08)
	aiScore = clamp(aiScore, 0.05, 0.95)

	plagiarismScore := clamp(rand.Float64()*0.12, 0.01, 0.12)

	var issues []string
	var flagged []string

	if aiScore >= m.AIThreshold {
		issues = []string{
			"high density of passive constructions",
			"sentence rhythm too uniform",
			"generic transitional phrases detected",
		}
		allSentences := strings.FieldsFunc(content, func(r rune) bool {
			return r == '.' || r == '!' || r == '?'
		})
		for i, s := range allSentences {
			s = strings.TrimSpace(s)
			if len(s) > 40 && i%3 == 0 {
				flagged = append(flagged, s+".")
			}
			if len(flagged) >= 3 {
				break
			}
		}
	}

	return &Result{
		AIScore:          round2(aiScore),
		PlagiarismScore:  round2(plagiarismScore),
		Original:         aiScore < m.AIThreshold,
		Provider:         "mock",
		Issues:           issues,
		SentencesFlagged: flagged,
		ReportURL:        "https://mock.originality.ai/report/mock-id-" + strings.ReplaceAll(strings.ToLower(strings.Fields(content)[0]), " ", "-"),
	}, nil
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func round2(v float64) float64 {
	return float64(int(v*100)) / 100
}
