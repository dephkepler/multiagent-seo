package checker

import (
	"context"
	"math/rand"
	"strings"
)

// Result holds the outcome of a content originality check.
type Result struct {
	AIScore          float64  `json:"ai_score"`                    // 0.0–1.0, fraction of AI-generated text
	PlagiarismScore  float64  `json:"plagiarism_score"`            // 0.0–1.0, fraction of plagiarised text
	Original         bool     `json:"original"`                    // true when both scores are below threshold
	Provider         string   `json:"provider"`                    // which service ran the check
	Issues           []string `json:"issues,omitempty"`            // human-readable reasons for failure
	SentencesFlagged []string `json:"sentences_flagged,omitempty"` // specific sentences flagged as AI-written
	ReportURL        string   `json:"report_url,omitempty"`        // link to full report on provider's site
}

// Client is the interface every checker implementation must satisfy.
// Swap MockClient for a real provider (Originality.ai, GPTZero, etc.)
// by implementing this interface and pointing CheckerConfig.Provider at it.
type Client interface {
	Check(ctx context.Context, content string) (*Result, error)
}

// MockClient always passes — used until a real provider is configured.
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
	// Simulate realistic AI detection scores based on content characteristics.
	// Longer texts with varied sentence structure score lower (more human-like).
	words := len(strings.Fields(content))
	sentences := strings.Count(content, ".") + strings.Count(content, "!") + strings.Count(content, "?")

	// Base AI score: shorter texts tend to score higher (less context = more AI-like).
	baseAI := 0.55
	if words > 1000 {
		baseAI -= 0.15
	} else if words > 500 {
		baseAI -= 0.08
	}

	// Varied sentence structure lowers the AI score.
	if sentences > 0 && words/sentences < 18 {
		baseAI -= 0.10
	}

	// Add small random variance ±0.08 to simulate real detector noise.
	aiScore := baseAI + (rand.Float64()*0.16 - 0.08)
	aiScore = clamp(aiScore, 0.05, 0.95)

	// Plagiarism score is typically low for AI-generated content.
	plagiarismScore := clamp(rand.Float64()*0.12, 0.01, 0.12)

	var issues []string
	var flagged []string

	if aiScore >= m.AIThreshold {
		issues = []string{
			"high density of passive constructions",
			"sentence rhythm too uniform",
			"generic transitional phrases detected",
		}
		// Pick a few sentences from the content to simulate flagging.
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
