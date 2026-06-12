package webfetch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"

	"multiagent-seo/internal/domain/linkbuilding"
)

const maxBody = 2 << 20

const maxTextSample = 2000

const userAgent = "Mozilla/5.0 (compatible; multiagent-seo-bot/1.0)"

const maxRedirects = 5

type Fetcher struct {
	http          *http.Client
	log           *slog.Logger
	allowLoopback bool
}

func New(log *slog.Logger) *Fetcher {
	if log == nil {
		log = slog.Default()
	}
	f := &Fetcher{log: log}
	f.http = &http.Client{
		Timeout:       15 * time.Second,
		Transport:     f.transport(),
		CheckRedirect: checkRedirect,
	}
	return f
}

func (f *Fetcher) transport() *http.Transport {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("webfetch dial guard: %w", err)
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("webfetch dial guard parse %q: %w", host, err)
			}
			if f.allowLoopback && ip.IsLoopback() {
				return nil
			}
			if disallowedIP(ip) {
				return fmt.Errorf("webfetch dial guard: blocked address %s", ip)
			}
			return nil
		},
	}
	t := &http.Transport{DialContext: dialer.DialContext}
	return t
}

func checkRedirect(_ *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("webfetch: stopped after %d redirects", maxRedirects)
	}
	return nil
}

func disallowedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	switch {
	case ip.IsLoopback(),
		ip.IsPrivate(),
		ip.IsLinkLocalUnicast(),
		ip.IsLinkLocalMulticast(),
		ip.IsMulticast(),
		ip.IsUnspecified(),
		ip.IsInterfaceLocalMulticast():
		return true
	}
	if ip.Is4() && cgnat.Contains(ip) {
		return true
	}
	return false
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (linkbuilding.Page, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return linkbuilding.Page{}, fmt.Errorf("webfetch parse url %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return linkbuilding.Page{}, fmt.Errorf("webfetch %q: unsupported scheme %q", rawURL, u.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return linkbuilding.Page{}, fmt.Errorf("webfetch new request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := f.http.Do(req)
	if err != nil {
		return linkbuilding.Page{}, fmt.Errorf("webfetch request %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return linkbuilding.Page{}, fmt.Errorf("webfetch %s: status %d", rawURL, resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mediaType, _, _ := strings.Cut(ct, ";")
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "text/html") {
			return linkbuilding.Page{}, fmt.Errorf("webfetch %s: unsupported content type %q", rawURL, ct)
		}
	}

	page, err := parsePage(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return linkbuilding.Page{}, fmt.Errorf("webfetch parse %s: %w", rawURL, err)
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

	page.TextSample = truncateRunes(text.String(), maxTextSample)

	return page, nil
}

func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
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
