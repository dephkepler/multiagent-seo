package llm

import "context"

// Client is the LLM provider interface. Swap groq for claude or any other provider.
// maxTokens caps output tokens; 0 means provider default.
type Client interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, error)
}
