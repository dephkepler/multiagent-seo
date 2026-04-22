package wordpress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"contentflow/internal/config"
)

type Client interface {
	CreateDraft(ctx context.Context, post Post) (int64, string, error)
	Publish(ctx context.Context, postID int64) (string, error)
}

type Post struct {
	Title    string
	Content  string
	SEOTitle string
	SEODesc  string
	Status   string
}

type client struct {
	cfg        config.WordPressConfig
	httpClient *http.Client
}

func New(cfg config.WordPressConfig) Client {
	return &client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type wpPost struct {
	Title   wpContent `json:"title"`
	Content wpContent `json:"content"`
	Status  string    `json:"status"`
}

type wpContent struct {
	Raw string `json:"raw"`
}

type wpResponse struct {
	ID   int64  `json:"id"`
	Link string `json:"link"`
}

// do marshals body, performs the HTTP request with Basic Auth, verifies the
// response status matches wantStatus, and decodes the JSON response into out.
// out may be nil if the caller does not need the decoded response.
func (c *client) do(ctx context.Context, method, url string, body any, wantStatus int, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal post: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("wordpress request failed: %w", err)
	}
	req.SetBasicAuth(c.cfg.User, c.cfg.AppPassword)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wordpress request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("wordpress returned %d: %s", resp.StatusCode, string(b))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *client) CreateDraft(ctx context.Context, post Post) (int64, string, error) {
	body := wpPost{
		Title:   wpContent{Raw: post.Title},
		Content: wpContent{Raw: post.Content},
		Status:  "draft",
	}

	url := c.cfg.URL + "/wp-json/wp/v2/posts"
	var result wpResponse
	if err := c.do(ctx, http.MethodPost, url, body, http.StatusCreated, &result); err != nil {
		return 0, "", err
	}

	editURL := fmt.Sprintf("%s/wp-admin/post.php?post=%d&action=edit", c.cfg.URL, result.ID)
	return result.ID, editURL, nil
}

func (c *client) Publish(ctx context.Context, postID int64) (string, error) {
	body := map[string]string{"status": "publish"}

	url := fmt.Sprintf("%s/wp-json/wp/v2/posts/%d", c.cfg.URL, postID)
	var result wpResponse
	if err := c.do(ctx, http.MethodPost, url, body, http.StatusOK, &result); err != nil {
		return "", err
	}

	return result.Link, nil
}
