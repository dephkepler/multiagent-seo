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

// FrontPage returns the static page set as the site's homepage, but only when it
// exists and the authenticated account may edit it. ok=false (with nil error)
// means there is no editable static homepage — a posts-index front page, a
// non-admin account that can't read settings, or a page we lack rights to.
func (c *Client) FrontPage(ctx context.Context, cred linkbuilding.DonorCredential) (linkbuilding.DonorPost, bool, error) {
	origin, err := siteOrigin(cred.DonorURL)
	if err != nil {
		return linkbuilding.DonorPost{}, false, err
	}

	var settings struct {
		ShowOnFront string `json:"show_on_front"`
		PageOnFront int64  `json:"page_on_front"`
	}
	st, body, err := c.get(ctx, cred, origin+"/wp-json/wp/v2/settings")
	if err != nil {
		return linkbuilding.DonorPost{}, false, fmt.Errorf("wppost settings: %w", err)
	}
	if st != http.StatusOK {
		return linkbuilding.DonorPost{}, false, nil
	}
	if err := json.Unmarshal(body, &settings); err != nil {
		return linkbuilding.DonorPost{}, false, nil
	}
	if settings.ShowOnFront != "page" || settings.PageOnFront <= 0 {
		return linkbuilding.DonorPost{}, false, nil
	}

	pageURL := fmt.Sprintf("%s/wp-json/wp/v2/pages/%d?context=edit&_fields=id,content,link", origin, settings.PageOnFront)
	st, body, err = c.get(ctx, cred, pageURL)
	if err != nil {
		return linkbuilding.DonorPost{}, false, fmt.Errorf("wppost front page: %w", err)
	}
	if st != http.StatusOK {
		return linkbuilding.DonorPost{}, false, nil
	}
	var page struct {
		ID      int64       `json:"id"`
		Content editContent `json:"content"`
		Link    string      `json:"link"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return linkbuilding.DonorPost{}, false, nil
	}
	return linkbuilding.DonorPost{
		ID:        page.ID,
		Content:   page.Content.Raw,
		PublicURL: page.Link,
		EditURL:   fmt.Sprintf("%s/wp-admin/post.php?post=%d&action=edit", origin, page.ID),
	}, true, nil
}

func (c *Client) UpdatePageContent(ctx context.Context, cred linkbuilding.DonorCredential, pageID int64, newHTML string) error {
	origin, err := siteOrigin(cred.DonorURL)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/wp-json/wp/v2/pages/%d", origin, pageID)
	st, body, err := c.postJSON(ctx, cred, u, map[string]string{"content": newHTML})
	if err != nil {
		return fmt.Errorf("wppost update page: %w", err)
	}
	if st/100 != 2 {
		return fmt.Errorf("wppost update page status %d: %s", st, snippet(body))
	}
	return nil
}

// LatestEditablePost returns the newest published post the account may edit:
// its own latest post when it is a low-privilege author, or the site's latest
// post when the account can edit any. ok=false means nothing is editable.
func (c *Client) LatestEditablePost(ctx context.Context, cred linkbuilding.DonorCredential) (linkbuilding.DonorPost, bool, error) {
	origin, err := siteOrigin(cred.DonorURL)
	if err != nil {
		return linkbuilding.DonorPost{}, false, err
	}

	meID, err := c.currentUserID(ctx, cred, origin)
	if err != nil {
		return linkbuilding.DonorPost{}, false, err
	}
	if meID > 0 {
		own := fmt.Sprintf("%s/wp-json/wp/v2/posts?author=%d&per_page=1&orderby=date&order=desc&status=publish&context=edit&_fields=id,content,link", origin, meID)
		if p, ok, err := c.postFromList(ctx, cred, origin, own); err != nil {
			return linkbuilding.DonorPost{}, false, err
		} else if ok {
			return p, true, nil
		}
	}

	id, ok, err := c.latestPostID(ctx, cred, origin)
	if err != nil || !ok {
		return linkbuilding.DonorPost{}, false, err
	}
	return c.postIfEditable(ctx, cred, origin, id)
}

func (c *Client) UpdatePostContent(ctx context.Context, cred linkbuilding.DonorCredential, postID int64, newHTML string) error {
	origin, err := siteOrigin(cred.DonorURL)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/wp-json/wp/v2/posts/%d", origin, postID)
	st, body, err := c.postJSON(ctx, cred, u, map[string]string{"content": newHTML})
	if err != nil {
		return fmt.Errorf("wppost update: %w", err)
	}
	if st/100 != 2 {
		return fmt.Errorf("wppost update status %d: %s", st, snippet(body))
	}
	return nil
}

// CreatePost publishes a new post. A Contributor account cannot publish, so on a
// publish-permission rejection it retries as a pending submission.
func (c *Client) CreatePost(ctx context.Context, cred linkbuilding.DonorCredential, title, html string) (linkbuilding.DonorPost, error) {
	origin, err := siteOrigin(cred.DonorURL)
	if err != nil {
		return linkbuilding.DonorPost{}, err
	}
	u := origin + "/wp-json/wp/v2/posts"

	post, err := c.createPostWithStatus(ctx, cred, origin, u, title, html, "publish")
	if err != nil && strings.Contains(err.Error(), "cannot_publish") {
		return c.createPostWithStatus(ctx, cred, origin, u, title, html, "pending")
	}
	return post, err
}

func (c *Client) createPostWithStatus(ctx context.Context, cred linkbuilding.DonorCredential, origin, u, title, html, status string) (linkbuilding.DonorPost, error) {
	st, body, err := c.postJSON(ctx, cred, u, map[string]string{"title": title, "content": html, "status": status})
	if err != nil {
		return linkbuilding.DonorPost{}, fmt.Errorf("wppost create: %w", err)
	}
	if st/100 != 2 {
		return linkbuilding.DonorPost{}, fmt.Errorf("wppost create status %d: %s", st, snippet(body))
	}
	var created struct {
		ID   int64  `json:"id"`
		Link string `json:"link"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		return linkbuilding.DonorPost{}, fmt.Errorf("wppost create decode: %w", err)
	}
	return linkbuilding.DonorPost{
		ID:        created.ID,
		PublicURL: created.Link,
		EditURL:   fmt.Sprintf("%s/wp-admin/post.php?post=%d&action=edit", origin, created.ID),
	}, nil
}

func (c *Client) currentUserID(ctx context.Context, cred linkbuilding.DonorCredential, origin string) (int64, error) {
	st, body, err := c.get(ctx, cred, origin+"/wp-json/wp/v2/users/me?_fields=id")
	if err != nil {
		return 0, fmt.Errorf("wppost users/me: %w", err)
	}
	if st != http.StatusOK {
		return 0, fmt.Errorf("wppost users/me status %d: %s", st, snippet(body))
	}
	var me struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return 0, fmt.Errorf("wppost users/me decode: %w", err)
	}
	return me.ID, nil
}

func (c *Client) latestPostID(ctx context.Context, cred linkbuilding.DonorCredential, origin string) (int64, bool, error) {
	u := origin + "/wp-json/wp/v2/posts?per_page=1&orderby=date&order=desc&status=publish&_fields=id"
	st, body, err := c.get(ctx, cred, u)
	if err != nil {
		return 0, false, fmt.Errorf("wppost list: %w", err)
	}
	if st != http.StatusOK {
		return 0, false, fmt.Errorf("wppost list status %d: %s", st, snippet(body))
	}
	var posts []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &posts); err != nil {
		return 0, false, fmt.Errorf("wppost decode list: %w", err)
	}
	if len(posts) == 0 {
		return 0, false, nil
	}
	return posts[0].ID, true, nil
}

// postIfEditable fetches a post in edit context; a 403 means the account cannot
// edit it, so ok=false lets the caller fall back to creating a new post.
func (c *Client) postIfEditable(ctx context.Context, cred linkbuilding.DonorCredential, origin string, id int64) (linkbuilding.DonorPost, bool, error) {
	u := fmt.Sprintf("%s/wp-json/wp/v2/posts/%d?context=edit&_fields=id,content,link", origin, id)
	st, body, err := c.get(ctx, cred, u)
	if err != nil {
		return linkbuilding.DonorPost{}, false, fmt.Errorf("wppost post: %w", err)
	}
	if st == http.StatusForbidden || st == http.StatusUnauthorized {
		return linkbuilding.DonorPost{}, false, nil
	}
	if st != http.StatusOK {
		return linkbuilding.DonorPost{}, false, fmt.Errorf("wppost post status %d: %s", st, snippet(body))
	}
	return decodePost(body, origin)
}

func (c *Client) postFromList(ctx context.Context, cred linkbuilding.DonorCredential, origin, u string) (linkbuilding.DonorPost, bool, error) {
	st, body, err := c.get(ctx, cred, u)
	if err != nil {
		return linkbuilding.DonorPost{}, false, fmt.Errorf("wppost own list: %w", err)
	}
	if st == http.StatusForbidden || st == http.StatusUnauthorized {
		return linkbuilding.DonorPost{}, false, nil
	}
	if st != http.StatusOK {
		return linkbuilding.DonorPost{}, false, fmt.Errorf("wppost own list status %d: %s", st, snippet(body))
	}
	var posts []json.RawMessage
	if err := json.Unmarshal(body, &posts); err != nil {
		return linkbuilding.DonorPost{}, false, fmt.Errorf("wppost decode own list: %w", err)
	}
	if len(posts) == 0 {
		return linkbuilding.DonorPost{}, false, nil
	}
	return decodePost(posts[0], origin)
}

func decodePost(raw []byte, origin string) (linkbuilding.DonorPost, bool, error) {
	var p struct {
		ID      int64       `json:"id"`
		Content editContent `json:"content"`
		Link    string      `json:"link"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return linkbuilding.DonorPost{}, false, fmt.Errorf("wppost decode post: %w", err)
	}
	return linkbuilding.DonorPost{
		ID:        p.ID,
		Content:   p.Content.Raw,
		PublicURL: p.Link,
		EditURL:   fmt.Sprintf("%s/wp-admin/post.php?post=%d&action=edit", origin, p.ID),
	}, true, nil
}

func (c *Client) get(ctx context.Context, cred linkbuilding.DonorCredential, u string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", basicAuth(cred.Login, cred.AppPassword))
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxPageBytes))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func (c *Client) postJSON(ctx context.Context, cred linkbuilding.DonorCredential, u string, payload any) (int, []byte, error) {
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("wppost marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAuth(cred.Login, cred.AppPassword))
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpx.MaxResponseBytes))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

type editContent struct {
	Raw string `json:"raw"`
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
