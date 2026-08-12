package modx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/pkg/httpx"
)

const maxLoggedBodyBytes = 4 << 10

var allowedContexts = map[string]bool{"uk": true, "web": true}

// Publisher talks to the site's own agentapi/publish.php over HTTP, which in
// turn uses MODX's native object API ($modx->newObject('modDocument')). That
// keeps alias/uri generation and the manager's own cache invalidation on the
// MODX side — a direct SQL write here would leave MODX's resource-URI cache
// stale, since nothing outside its own PHP process can refresh it.
type Publisher struct {
	siteURL    string
	apiKey     string
	context    string
	httpClient *http.Client
	log        *slog.Logger
}

func New(siteURL, apiKey, context string, log *slog.Logger, timeout time.Duration) (articles.Publisher, error) {
	if !allowedContexts[context] {
		return nil, fmt.Errorf("modx: unknown context %q", context)
	}
	return &Publisher{
		siteURL:    trimTrailingSlash(siteURL),
		apiKey:     apiKey,
		context:    context,
		httpClient: httpx.New(httpx.WithTimeout(timeout), httpx.BlockPrivateIPs()),
		log:        log,
	}, nil
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

type publishRequest struct {
	Action    string            `json:"action"`
	PageTitle string            `json:"pagetitle,omitempty"`
	Content   string            `json:"content,omitempty"`
	Context   string            `json:"context,omitempty"`
	Alias     string            `json:"alias,omitempty"`
	ID        int64             `json:"id,omitempty"`
	TV        map[string]string `json:"tv,omitempty"`
}

type publishResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	ID      int64  `json:"id"`
	URL     string `json:"url"`
}

func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

func (p *Publisher) call(ctx context.Context, req publishRequest) (publishResponse, error) {
	buf, err := json.Marshal(req)
	if err != nil {
		return publishResponse{}, fmt.Errorf("modx: marshal request: %w", err)
	}

	url := p.siteURL + "/agentapi/publish.php"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return publishResponse{}, fmt.Errorf("modx: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", p.apiKey)

	start := time.Now()
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return publishResponse{}, fmt.Errorf("modx: request: %w", err)
	}
	defer resp.Body.Close()

	p.log.DebugContext(ctx, "modx request",
		"action", req.Action,
		"status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	limited := io.LimitReader(resp.Body, httpx.MaxResponseBytes)
	b, err := io.ReadAll(limited)
	if err != nil {
		return publishResponse{}, fmt.Errorf("modx: read response: %w", err)
	}

	var result publishResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return publishResponse{}, fmt.Errorf("modx: decode response (status %d): %s", resp.StatusCode, truncate(b, maxLoggedBodyBytes))
	}
	if !result.Success {
		return publishResponse{}, fmt.Errorf("modx: %s returned %d: %s", req.Action, resp.StatusCode, result.Error)
	}
	return result, nil
}

func (p *Publisher) CreateDraft(ctx context.Context, post articles.Post) (int64, string, error) {
	tv := map[string]string{}
	if post.SEOTitle != "" {
		tv["meta.title"] = post.SEOTitle
	}
	if post.SEODesc != "" {
		tv["meta.description"] = post.SEODesc
	}

	result, err := p.call(ctx, publishRequest{
		Action:    "create_draft",
		PageTitle: post.Title,
		Content:   post.Content,
		Context:   p.context,
		Alias:     slugify(post.Title),
		TV:        tv,
	})
	if err != nil {
		return 0, "", err
	}

	editURL := fmt.Sprintf("%s/manager/?a=resource/update&id=%d", p.siteURL, result.ID)
	return result.ID, editURL, nil
}

func (p *Publisher) Publish(ctx context.Context, postID int64) (string, error) {
	result, err := p.call(ctx, publishRequest{Action: "publish", ID: postID})
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

// Delete soft-deletes a resource (MODX's own trash, restorable from the
// Manager) — never used on the standard generate-and-publish path, only for
// cleaning up manual/test articles. Not part of the articles.Publisher
// interface, called directly on the concrete type.
func (p *Publisher) Delete(ctx context.Context, postID int64) error {
	_, err := p.call(ctx, publishRequest{Action: "delete", ID: postID})
	return err
}
