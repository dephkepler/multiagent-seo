// Package llm adapts the provider HTTP clients to the generate.LLMClient
// domain port. Provider codecs return a transport.Client; client here wraps it
// to translate transport usage counts into the domain Usage type.
package llm

import (
	"context"

	"multiagent-seo/internal/domain/generate"
	"multiagent-seo/internal/infrastructure/llm/transport"
)

var _ generate.LLMClient = (*client)(nil)

type client struct {
	transport *transport.Client
}

func (c *client) Complete(ctx context.Context, prompt string, maxTokens int) (string, generate.Usage, error) {
	content, u, err := c.transport.Complete(ctx, prompt, maxTokens)
	if err != nil {
		return "", generate.Usage{}, err
	}
	return content, generate.Usage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
	}, nil
}
