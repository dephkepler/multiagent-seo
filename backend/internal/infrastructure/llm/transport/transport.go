// Package transport executes provider-agnostic LLM HTTP calls with retries,
// logging, bounded reads, and error classification. Provider-specific wire
// formats live behind the Codec interface.
package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"multiagent-seo/internal/infrastructure/llm/retry"
	"multiagent-seo/internal/infrastructure/llm/usage"
)

// Cap on response read; provider JSON is tiny, so 1 MiB guards against runaway bodies.
const maxResponseBytes = 1 << 20

// Cap on the error-body snippet so logs/errors don't carry up to maxResponseBytes.
const maxErrorBodyBytes = 4 << 10

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

// truncateBody caps an error-response snippet at maxErrorBodyBytes.
func truncateBody(body []byte) string {
	if len(body) <= maxErrorBodyBytes {
		return string(body)
	}
	return string(body[:maxErrorBodyBytes]) + "...(truncated)"
}

// isTruncated reports whether a provider stop/finish reason indicates a max_tokens cutoff.
func isTruncated(reason string) bool {
	return reason == "max_tokens" || reason == "length"
}

func (c *Client) Complete(ctx context.Context, prompt string, maxTokens int) (string, usage.Usage, error) {
	provider := c.codec.Provider()

	c.log.DebugContext(ctx, "llm call start",
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
			return &httpError{provider: provider, status: resp.StatusCode, body: truncateBody(body)}
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
		// Terminal 4xx is a caller/config fault (bad model/input), not a service
		// failure; log Warn. Retryable-exhausted/5xx/transport (status 0) → Error.
		logFailure := c.log.ErrorContext
		var he retry.HTTPStatusError
		if errors.As(err, &he) {
			if s := he.HTTPStatus(); s >= 400 && s < 500 {
				logFailure = c.log.WarnContext
			}
		}
		logFailure(ctx, "llm call done",
			"provider", provider,
			"model", c.model,
			"status", lastStatus,
			"latency_ms", latency.Milliseconds(),
			"input_tokens", u.InputTokens,
			"output_tokens", u.OutputTokens,
			"err", err,
		)
		return "", usage.Usage{}, fmt.Errorf("%s request failed (model %s): %w", provider, c.model, err)
	}

	if isTruncated(u.FinishReason) {
		c.log.WarnContext(ctx, "llm response truncated",
			"provider", provider,
			"model", c.model,
			"finish_reason", u.FinishReason,
			"output_tokens", u.OutputTokens,
		)
	}

	c.log.InfoContext(ctx, "llm call done",
		"provider", provider,
		"model", c.model,
		"status", lastStatus,
		"latency_ms", latency.Milliseconds(),
		"input_tokens", u.InputTokens,
		"output_tokens", u.OutputTokens,
	)
	return content, u, nil
}
