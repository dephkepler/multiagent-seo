package wppost

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/httpx"
)

type Client struct {
	http *http.Client
	log  *slog.Logger
}

func New(log *slog.Logger, timeout time.Duration) *Client {
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		http: httpx.New(
			httpx.WithTimeout(timeout),
			httpx.BlockPrivateIPs(),
			httpx.InsecureTLS(),
		),
		log: log,
	}
}

func (c *Client) LatestPost(ctx context.Context, cred linkbuilding.DonorCredential) (linkbuilding.DonorPost, error) {
	origin, err := siteOrigin(cred.DonorURL)
	if err != nil {
		return linkbuilding.DonorPost{}, err
	}
	u := origin + "/wp-json/wp/v2/posts?per_page=1&orderby=date&order=desc&status=publish&_fields=id,title,content,link"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return linkbuilding.DonorPost{}, fmt.Errorf("wppost new request: %w", err)
	}
	req.Header.Set("Authorization", basicAuth(cred.Login, cred.AppPassword))

	resp, err := c.http.Do(req)
	if err != nil {
		return linkbuilding.DonorPost{}, fmt.Errorf("wppost list: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxPageBytes))
	if err != nil {
		return linkbuilding.DonorPost{}, fmt.Errorf("wppost list read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return linkbuilding.DonorPost{}, fmt.Errorf("wppost list status %d: %s", resp.StatusCode, snippet(body))
	}

	var posts []struct {
		ID      int64         `json:"id"`
		Title   renderedField `json:"title"`
		Content renderedField `json:"content"`
		Link    string        `json:"link"`
	}
	if err := json.Unmarshal(body, &posts); err != nil {
		return linkbuilding.DonorPost{}, fmt.Errorf("wppost decode list: %w", err)
	}
	if len(posts) == 0 {
		return linkbuilding.DonorPost{}, fmt.Errorf("wppost: no published posts")
	}
	p := posts[0]
	return linkbuilding.DonorPost{
		ID:        p.ID,
		Title:     p.Title.Rendered,
		Content:   p.Content.Rendered,
		PublicURL: p.Link,
		EditURL:   fmt.Sprintf("%s/wp-admin/post.php?post=%d&action=edit", origin, p.ID),
	}, nil
}

func (c *Client) UpdatePostContent(ctx context.Context, cred linkbuilding.DonorCredential, postID int64, newHTML string) error {
	origin, err := siteOrigin(cred.DonorURL)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/wp-json/wp/v2/posts/%d", origin, postID)

	bodyJSON, err := json.Marshal(map[string]string{"content": newHTML})
	if err != nil {
		return fmt.Errorf("wppost marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return fmt.Errorf("wppost new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAuth(cred.Login, cred.AppPassword))

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wppost update: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxResponseBytes))
	if err != nil {
		return fmt.Errorf("wppost update read response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("wppost update status %d: %s", resp.StatusCode, snippet(body))
	}
	return nil
}

type renderedField struct {
	Rendered string `json:"rendered"`
}

func basicAuth(login, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(login+":"+password))
}

func siteOrigin(s string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(s), "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid donor URL %q", s)
	}
	return u.Scheme + "://" + u.Host, nil
}

func snippet(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
