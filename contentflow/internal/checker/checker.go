package checker

import "context"

// Result holds the outcome of a content originality check.
type Result struct {
	AIScore         float64  `json:"ai_score"`          // 0.0–1.0, fraction of AI-generated text
	PlagiarismScore float64  `json:"plagiarism_score"`  // 0.0–1.0, fraction of plagiarised text
	Original        bool     `json:"original"`          // true when both scores are below threshold
	Provider        string   `json:"provider"`          // which service ran the check
	Issues          []string `json:"issues,omitempty"`  // human-readable reasons for failure
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

func (m *MockClient) Check(_ context.Context, _ string) (*Result, error) {
	score := 0.12
	return &Result{
		AIScore:         score,
		PlagiarismScore: 0.03,
		Original:        score < m.AIThreshold,
		Provider:        "mock",
	}, nil
}
