package llm

import (
	"context"

	"contentflow/internal/llm/usage"
)

// Usage is an alias so callers use llm.Usage without knowing the subpackage.
type Usage = usage.Usage

// Client is the LLM provider interface. Swap groq for claude or any other provider.
// maxTokens caps output tokens; 0 means provider default.
type Client interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, Usage, error)
}
