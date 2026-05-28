// Package wordpress implements publisher.Publisher against the WordPress REST API.
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

	"multiagent-seo/internal/config"
	"multiagent-seo/internal/publisher"
)

// maxResponseBytes guards against a misbehaving proxy or server; real WP
// REST replies we use are tiny JSON objects.
const maxResponseBytes = 1 << 20

type Client struct {
	cfg        config.WordPressConfig
	httpClient *http.Client
	log        *slog.Logger
}

func New(cfg config.WordPressConfig, log *slog.Logger) publisher.Publisher {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		log: log,
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

// do issues the request with Basic Auth and decodes into out (may be nil).
func (c *Client) do(ctx context.Context, method, url string, body any, wantStatus int, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal post: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("wordpress request: %w", err)
	}
	req.SetBasicAuth(c.cfg.User, c.cfg.AppPassword)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("wordpress request failed",
			"method", method,
			"url", url,
			"duration_ms", time.Since(start).Milliseconds(),
			"err", err,
		)
		return fmt.Errorf("wordpress request: %w", err)
	}
	defer resp.Body.Close()

	durationMS := time.Since(start).Milliseconds()
	c.log.Info("wordpress request",
		"method", method,
		"url", url,
		"status", resp.StatusCode,
		"duration_ms", durationMS,
	)

	// Cap reads so a runaway response can't blow memory.
	body_ := io.LimitReader(resp.Body, maxResponseBytes)

	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(body_)
		return fmt.Errorf("wordpress returned %d: %s", resp.StatusCode, string(b))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(body_).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) CreateDraft(ctx context.Context, post publisher.Post) (int64, string, error) {
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

	url := c.cfg.URL + "/wp-json/wp/v2/posts"
	var result wpResponse
	if err := c.do(ctx, http.MethodPost, url, body, http.StatusCreated, &result); err != nil {
		return 0, "", err
	}

	editURL := fmt.Sprintf("%s/wp-admin/post.php?post=%d&action=edit", c.cfg.URL, result.ID)
	return result.ID, editURL, nil
}

// seoMeta returns Yoast (and Rank Math) compatible meta keys. WP silently
// ignores meta keys the active SEO plugin doesn't register, so sending
// both vendors is harmless if only one is installed (or neither — then
// the meta block is a no-op).
func seoMeta(post publisher.Post) map[string]any {
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

func (c *Client) Publish(ctx context.Context, postID int64) (string, error) {
	body := map[string]string{"status": "publish"}

	url := fmt.Sprintf("%s/wp-json/wp/v2/posts/%d", c.cfg.URL, postID)
	var result wpResponse
	if err := c.do(ctx, http.MethodPost, url, body, http.StatusOK, &result); err != nil {
		return "", err
	}

	return result.Link, nil
}
