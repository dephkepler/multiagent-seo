package llm

import (
	"context"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/infrastructure/llm/transport"
)

type client struct {
	transport *transport.Client
}

func (c *client) Complete(ctx context.Context, prompt string, maxTokens int) (string, articles.Usage, error) {
	content, u, err := c.transport.Complete(ctx, prompt, maxTokens)
	if err != nil {
		return "", articles.Usage{}, err
	}
	return content, articles.Usage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
	}, nil
}
