package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"multiagent-seo/internal/infrastructure/llm/transport"
	"multiagent-seo/internal/infrastructure/llm/usage"
)

const (
	apiURL       = "https://api.groq.com/openai/v1/chat/completions"
	providerName = "groq"
)

func New(key, model string, log *slog.Logger) *transport.Client {
	return transport.New(&codec{key: key, model: model}, model, log)
}

type codec struct {
	key   string
	model string
}

func (c *codec) Provider() string { return providerName }

func (c *codec) BuildRequest(ctx context.Context, prompt string, maxTokens int) (*http.Request, error) {
	reqBody := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if maxTokens > 0 {
		reqBody["max_tokens"] = maxTokens
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *codec) ParseResponse(body []byte) (string, usage.Usage, error) {
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", usage.Usage{}, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", usage.Usage{}, fmt.Errorf("groq returned empty response")
	}
	return result.Choices[0].Message.Content, usage.Usage{
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
	}, nil
}
