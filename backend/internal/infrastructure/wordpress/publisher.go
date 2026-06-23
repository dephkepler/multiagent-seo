package wordpress

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

type Publisher struct {
	url         string
	username    string
	appPassword string
	siteID      string
	httpClient  *http.Client
	log         *slog.Logger
}

func New(url, username, appPassword, siteID string, log *slog.Logger, timeout time.Duration) articles.Publisher {
	return &Publisher{
		url:         url,
		username:    username,
		appPassword: appPassword,
		siteID:      siteID,
		httpClient:  httpx.New(httpx.WithTimeout(timeout), httpx.BlockPrivateIPs()),
		log:         log,
	}
}

type wpPost struct {
	Title   wpContent      `json:"title"`
	Content wpContent      `json:"content"`
	Status  string         `json:"status"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type wpContent struct {
	Raw string `json:"raw"`
}

type wpResponse struct {
	ID   int64  `json:"id"`
	Link string `json:"link"`
}

func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

func (p *Publisher) request(ctx context.Context, method, url string, body any, wantStatus int, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal post: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("wordpress request: %w", err)
	}
	req.SetBasicAuth(p.username, p.appPassword)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wordpress request: %w", err)
	}
	defer resp.Body.Close()

	p.log.DebugContext(ctx, "wordpress request",
		"site_id", p.siteID,
		"method", method,
		"url", url,
		"status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	limited := io.LimitReader(resp.Body, httpx.MaxResponseBytes)

	if resp.StatusCode != wantStatus {
		b, err := io.ReadAll(limited)
		if err != nil {
			return fmt.Errorf("wordpress returned %d, read response: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("wordpress returned %d: %s", resp.StatusCode, truncate(b, maxLoggedBodyBytes))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(limited).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (p *Publisher) CreateDraft(ctx context.Context, post articles.Post) (int64, string, error) {
	status := post.Status
	if status == "" {
		status = "draft"
	}
	body := wpPost{
		Title:   wpContent{Raw: post.Title},
		Content: wpContent{Raw: post.Content},
		Status:  status,
		Meta:    seoMeta(post),
	}

	url := p.url + "/wp-json/wp/v2/posts"
	var result wpResponse
	if err := p.request(ctx, http.MethodPost, url, body, http.StatusCreated, &result); err != nil {
		return 0, "", err
	}

	editURL := fmt.Sprintf("%s/wp-admin/post.php?post=%d&action=edit", p.url, result.ID)
	return result.ID, editURL, nil
}

func seoMeta(post articles.Post) map[string]any {
	if post.SEOTitle == "" && post.SEODesc == "" {
		return nil
	}
	m := map[string]any{}
	if post.SEOTitle != "" {
		m["_yoast_wpseo_title"] = post.SEOTitle
		m["rank_math_title"] = post.SEOTitle
	}
	if post.SEODesc != "" {
		m["_yoast_wpseo_metadesc"] = post.SEODesc
		m["rank_math_description"] = post.SEODesc
	}
	return m
}

func (p *Publisher) Publish(ctx context.Context, postID int64) (string, error) {
	body := map[string]string{"status": "publish"}

	url := fmt.Sprintf("%s/wp-json/wp/v2/posts/%d", p.url, postID)
	var result wpResponse
	if err := p.request(ctx, http.MethodPost, url, body, http.StatusOK, &result); err != nil {
		return "", err
	}

	return result.Link, nil
}
