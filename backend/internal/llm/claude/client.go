package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"multiagent-seo/internal/llm/transport"
	"multiagent-seo/internal/llm/usage"
)

const (
	apiURL           = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	providerName     = "claude"

	// Anthropic's Messages API requires max_tokens; sized for a full article draft.
	defaultMaxTokens = 4096
)

// Returns *transport.Client (not llm.Client) to avoid an import cycle with package llm.
func New(key, model string, log *slog.Logger) *transport.Client {
	return transport.New(&codec{key: key, model: model}, model, log)
}

type codec struct {
	key   string
	model string
}

func (c *codec) Provider() string { return providerName }

func (c *codec) BuildRequest(ctx context.Context, prompt string, maxTokens int) (*http.Request, error) {
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	body, err := json.Marshal(map[string]any{
		"model":      c.model,
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.key)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *codec) ParseResponse(body []byte) (string, usage.Usage, error) {
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", usage.Usage{}, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", usage.Usage{}, fmt.Errorf("claude returned empty response")
	}
	return result.Content[0].Text, usage.Usage{
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}, nil
}
