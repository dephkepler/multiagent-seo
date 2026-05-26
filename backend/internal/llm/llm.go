package llm

import (
	"context"

	"contentflow/internal/llm/usage"
)

type Usage = usage.Usage

// Client is the LLM provider interface. maxTokens == 0 means provider default.
type Client interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, Usage, error)
}
