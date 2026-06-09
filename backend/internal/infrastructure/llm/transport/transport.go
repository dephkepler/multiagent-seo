package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"multiagent-seo/internal/infrastructure/llm/retry"
	"multiagent-seo/internal/infrastructure/llm/usage"
)

const maxResponseBytes = 1 << 20

const maxErrorBodyBytes = 4 << 10

const requestTimeout = 180 * time.Second

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

type httpError struct {
	provider   string
	status     int
	body       string
	retryAfter time.Duration
}

func (e *httpError) Error() string {
	if e.status == 0 {
		return e.provider + " transport error: " + e.body
	}
	return fmt.Sprintf("%s returned %d: %s", e.provider, e.status, e.body)
}

func (e *httpError) HTTPStatus() int { return e.status }

func (e *httpError) RetryAfter() (time.Duration, bool) { return e.retryAfter, e.retryAfter > 0 }

func parseRetryAfter(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil && secs > 0 {
		return time.Duration(secs * float64(time.Second))
	}
	return 0
}

func truncateBody(body []byte) string {
	if len(body) <= maxErrorBodyBytes {
		return string(body)
	}
	return string(body[:maxErrorBodyBytes]) + "...(truncated)"
}

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
			return &httpError{provider: provider, status: resp.StatusCode, body: truncateBody(body), retryAfter: parseRetryAfter(resp.Header)}
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
