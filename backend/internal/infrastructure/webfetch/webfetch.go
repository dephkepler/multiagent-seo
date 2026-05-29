// Package webfetch is an infrastructure adapter that fetches a site's homepage
// over HTTP and parses it into the signals qualification needs, implementing
// the domain linkbuilding.PageFetcher port.
package webfetch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	"multiagent-seo/internal/domain/linkbuilding"
)

// maxBody caps the bytes read from a response so a hostile or huge page can't
// exhaust memory; ~2 MiB is far more than any homepage's worth of HTML signal.
const maxBody = 2 << 20

// maxTextSample caps the visible-text sample passed to the classifier — enough
// to identify the topic without shipping the whole page to an LLM.
const maxTextSample = 2000

// userAgent overrides the default Go UA, which many sites 403.
const userAgent = "Mozilla/5.0 (compatible; multiagent-seo-bot/1.0)"

type Fetcher struct {
	http *http.Client
	log  *slog.Logger
}

var _ linkbuilding.PageFetcher = (*Fetcher)(nil)

func New(log *slog.Logger) *Fetcher {
	if log == nil {
		log = slog.Default()
	}
	return &Fetcher{
		http: &http.Client{Timeout: 15 * time.Second},
		log:  log,
	}
}

func (f *Fetcher) Fetch(ctx context.Context, url string) (linkbuilding.Page, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return linkbuilding.Page{}, fmt.Errorf("webfetch new request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := f.http.Do(req)
	if err != nil {
		return linkbuilding.Page{}, fmt.Errorf("webfetch request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		f.log.WarnContext(ctx, "webfetch non-2xx", "url", url, "status", resp.StatusCode)
		return linkbuilding.Page{}, fmt.Errorf("webfetch %s: status %d", url, resp.StatusCode)
	}

	page, err := parsePage(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return linkbuilding.Page{}, fmt.Errorf("webfetch parse %s: %w", url, err)
	}
	return page, nil
}

func parsePage(body io.Reader) (linkbuilding.Page, error) {
	root, err := html.Parse(body)
	if err != nil {
		return linkbuilding.Page{}, fmt.Errorf("parse html: %w", err)
	}

	var page linkbuilding.Page
	var text strings.Builder

	var walk func(n *html.Node, inSkip bool)
	walk = func(n *html.Node, inSkip bool) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript":
				inSkip = true
			case "title":
				page.Title = strings.TrimSpace(nodeText(n))
			case "meta":
				if content, ok := metaDescription(n); ok {
					page.MetaDescription = content
				}
			case "h1", "h2", "h3":
				if h := strings.TrimSpace(nodeText(n)); h != "" {
					page.Headings = append(page.Headings, h)
				}
			case "a":
				if href, ok := attr(n, "href"); ok {
					page.Links = append(page.Links, href)
				}
			}
		}

		if !inSkip && n.Type == html.TextNode {
			if t := strings.TrimSpace(n.Data); t != "" && text.Len() < maxTextSample {
				if text.Len() > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(t)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inSkip)
		}
	}
	walk(root, false)

	sample := text.String()
	if len(sample) > maxTextSample {
		sample = sample[:maxTextSample]
	}
	page.TextSample = sample

	return page, nil
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		} else if c.Type == html.ElementNode {
			b.WriteString(nodeText(c))
		}
	}
	return b.String()
}

func attr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func metaDescription(n *html.Node) (content string, ok bool) {
	if name, has := attr(n, "name"); !has || !strings.EqualFold(name, "description") {
		return "", false
	}
	c, _ := attr(n, "content")
	return strings.TrimSpace(c), true
}
