package pagefetch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"multiagent-seo/internal/domain/emailscrape"
	"multiagent-seo/pkg/httpx"
)

const maxBody = 2 << 20

type Fetcher struct {
	http *http.Client
	log  *slog.Logger
}

func New(log *slog.Logger) *Fetcher {
	if log == nil {
		log = slog.Default()
	}
	return &Fetcher{
		log: log,
		http: httpx.New(
			httpx.WithTimeout(15*time.Second),
			httpx.WithMaxRedirects(5),
			httpx.BlockPrivateIPs(),
		),
	}
}

// Fetch returns a page's full HTML (capped) and its anchor hrefs. Unlike the
// link-building fetcher it keeps the whole body, since emails can sit anywhere.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (emailscrape.Page, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return emailscrape.Page{}, fmt.Errorf("pagefetch parse url %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return emailscrape.Page{}, fmt.Errorf("pagefetch %q: unsupported scheme %q", rawURL, u.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return emailscrape.Page{}, fmt.Errorf("pagefetch new request: %w", err)
	}

	resp, err := f.http.Do(req)
	if err != nil {
		return emailscrape.Page{}, fmt.Errorf("pagefetch request %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return emailscrape.Page{}, fmt.Errorf("pagefetch %s: status %d", rawURL, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mediaType, _, _ := strings.Cut(ct, ";")
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "text/html") {
			return emailscrape.Page{}, fmt.Errorf("pagefetch %s: unsupported content type %q", rawURL, ct)
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return emailscrape.Page{}, fmt.Errorf("pagefetch read %s: %w", rawURL, err)
	}

	return emailscrape.Page{URL: rawURL, HTML: string(body), Links: extractLinks(string(body))}, nil
}

func extractLinks(body string) []emailscrape.Link {
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}
	var links []emailscrape.Link
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			if href, ok := attr(n, "href"); ok && href != "" {
				links = append(links, emailscrape.Link{Href: href, Text: nodeText(n)})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return links
}

func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}
