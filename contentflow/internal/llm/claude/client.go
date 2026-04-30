package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"contentflow/internal/llm/retry"
	"contentflow/internal/llm/usage"
)

const (
	apiURL         = "https://api.anthropic.com/v1/messages"
	providerName   = "claude"
	requestTimeout = 180 * time.Second
)

// Client is an Anthropic Messages API client with retry + structured logging.
type Client struct {
	key        string
	model      string
	httpClient *http.Client
	log        *slog.Logger
	retryCfg   retry.Config
}

// New constructs a Claude client. The returned *Client satisfies llm.Client
// structurally; the factory in package llm hands it back as llm.Client.
// Returning a concrete type here (rather than llm.Client) keeps the claude
// package free of a back-import of llm — which would create an import cycle
// with internal/llm/factory.go.
//
// If log is nil, slog.Default() is used.
func New(key, model string, log *slog.Logger) *Client {
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		key:   key,
		model: model,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
		log:      log,
		retryCfg: retry.Default(),
	}
}

// httpError implements retry.HTTPStatusError so retry.Do can classify failures.
type httpError struct {
	status int
	body   string
}

func (e *httpError) Error() string {
	if e.status == 0 {
		return "claude transport error: " + e.body
	}
	return fmt.Sprintf("claude returned %d: %s", e.status, e.body)
}

func (e *httpError) HTTPStatus() int { return e.status }

// Complete sends a single Messages API request, retrying on 429/5xx and
// transport errors per the configured retry policy. maxTokens caps output; 0 uses 4096 (Anthropic requires the field).
func (c *Client) Complete(ctx context.Context, prompt string, maxTokens int) (string, usage.Usage, error) {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	c.log.Debug("llm call start",
		"provider", providerName,
		"model", c.model,
		"prompt_len", len(prompt),
		"max_tokens", maxTokens,
	)

	body, err := json.Marshal(map[string]any{
		"model":      c.model,
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})
	if err != nil {
		return "", usage.Usage{}, err
	}

	var (
		content    string
		lastStatus int
		u          usage.Usage
	)

	start := time.Now()
	err = retry.Do(ctx, c.retryCfg, c.log, providerName, func() error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("x-api-key", c.key)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("Content-Type", "application/json")

		resp, doErr := c.httpClient.Do(req)
		if doErr != nil {
			return &httpError{status: 0, body: doErr.Error()}
		}
		defer resp.Body.Close()

		lastStatus = resp.StatusCode
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return &httpError{status: resp.StatusCode, body: string(b)}
		}

		var result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if decErr := json.NewDecoder(resp.Body).Decode(&result); decErr != nil {
			return fmt.Errorf("decode response: %w", decErr)
		}
		if len(result.Content) == 0 {
			return fmt.Errorf("claude returned empty response")
		}
		content = result.Content[0].Text
		u = usage.Usage{
			InputTokens:  result.Usage.InputTokens,
			OutputTokens: result.Usage.OutputTokens,
		}
		return nil
	})

	latency := time.Since(start)

	if err != nil {
		c.log.Error("llm call done",
			"provider", providerName,
			"model", c.model,
			"status", lastStatus,
			"latency_ms", latency.Milliseconds(),
			"input_tokens", u.InputTokens,
			"output_tokens", u.OutputTokens,
			"error", err,
		)
		return "", usage.Usage{}, fmt.Errorf("claude request failed: %w", err)
	}

	c.log.Info("llm call done",
		"provider", providerName,
		"model", c.model,
		"status", lastStatus,
		"latency_ms", latency.Milliseconds(),
		"input_tokens", u.InputTokens,
		"output_tokens", u.OutputTokens,
	)
	return content, u, nil
}
