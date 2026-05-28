// Package transport executes provider-agnostic LLM HTTP calls with retries,
// logging, bounded reads, and error classification. Provider-specific wire
// formats live behind the Codec interface.
package transport

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"multiagent-seo/internal/llm/retry"
	"multiagent-seo/internal/llm/usage"
)

// Cap on response read; provider JSON is tiny, so 1 MiB guards against runaway bodies.
const maxResponseBytes = 1 << 20

// Per-attempt HTTP timeout; retry.Do may make several attempts within the caller's deadline.
const requestTimeout = 180 * time.Second

// BuildRequest is called once per retry so each attempt gets a fresh Request bound to ctx.
type Codec interface {
	Provider() string
	BuildRequest(ctx context.Context, prompt string, maxTokens int) (*http.Request, error)
	ParseResponse(body []byte) (string, usage.Usage, error)
}

type Client struct {
	codec      Codec
	model      string
	httpClient *http.Client
	log        *slog.Logger
	retryCfg   retry.Config
}

// model is logged for observability only; the codec controls what goes on the wire.
func New(codec Codec, model string, log *slog.Logger) *Client {
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		codec:      codec,
		model:      model,
		httpClient: &http.Client{Timeout: requestTimeout},
		log:        log,
		retryCfg:   retry.Default(),
	}
}

// Implements retry.HTTPStatusError; status 0 means transport-level failure.
type httpError struct {
	provider string
	status   int
	body     string
}

func (e *httpError) Error() string {
	if e.status == 0 {
		return e.provider + " transport error: " + e.body
	}
	return fmt.Sprintf("%s returned %d: %s", e.provider, e.status, e.body)
}

func (e *httpError) HTTPStatus() int { return e.status }

func (c *Client) Complete(ctx context.Context, prompt string, maxTokens int) (string, usage.Usage, error) {
	provider := c.codec.Provider()

	c.log.Debug("llm call start",
		"provider", provider,
		"model", c.model,
		"prompt_len", len(prompt),
		"max_tokens", maxTokens,
	)

	var (
		content    string
		u          usage.Usage
		lastStatus int
	)

	start := time.Now()
	err := retry.Do(ctx, c.retryCfg, c.log, provider, func() error {
		req, reqErr := c.codec.BuildRequest(ctx, prompt, maxTokens)
		if reqErr != nil {
			return reqErr
		}

		resp, doErr := c.httpClient.Do(req)
		if doErr != nil {
			return &httpError{provider: provider, status: 0, body: doErr.Error()}
		}
		defer resp.Body.Close()

		lastStatus = resp.StatusCode
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		if readErr != nil {
			return &httpError{provider: provider, status: resp.StatusCode, body: readErr.Error()}
		}

		if resp.StatusCode != http.StatusOK {
			return &httpError{provider: provider, status: resp.StatusCode, body: string(body)}
		}

		parsedContent, parsedUsage, parseErr := c.codec.ParseResponse(body)
		if parseErr != nil {
			return parseErr
		}
		content = parsedContent
		u = parsedUsage
		return nil
	})

	latency := time.Since(start)

	if err != nil {
		c.log.Error("llm call done",
			"provider", provider,
			"model", c.model,
			"status", lastStatus,
			"latency_ms", latency.Milliseconds(),
			"input_tokens", u.InputTokens,
			"output_tokens", u.OutputTokens,
			"err", err,
		)
		return "", usage.Usage{}, fmt.Errorf("%s request failed: %w", provider, err)
	}

	c.log.Info("llm call done",
		"provider", provider,
		"model", c.model,
		"status", lastStatus,
		"latency_ms", latency.Milliseconds(),
		"input_tokens", u.InputTokens,
		"output_tokens", u.OutputTokens,
	)
	return content, u, nil
}
