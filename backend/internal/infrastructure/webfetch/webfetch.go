// Package webfetch is an infrastructure adapter that fetches a site's homepage
// over HTTP and parses it into the signals qualification needs, implementing
// the domain linkbuilding.PageFetcher port.
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

// maxBody caps the bytes read from a response so a hostile or huge page can't
// exhaust memory; ~2 MiB is far more than any homepage's worth of HTML signal.
const maxBody = 2 << 20

// maxTextSample caps the visible-text sample passed to the classifier — enough
// to identify the topic without shipping the whole page to an LLM.
const maxTextSample = 2000

// userAgent overrides the default Go UA, which many sites 403.
const userAgent = "Mozilla/5.0 (compatible; multiagent-seo-bot/1.0)"

// maxRedirects caps redirect hops; the dial-time IP guard still vets every hop,
// this just stops redirect loops.
const maxRedirects = 5

type Fetcher struct {
	http *http.Client
	log  *slog.Logger
	// allowLoopback relaxes the loopback rejection so tests can hit httptest
	// servers (which bind 127.0.0.1); it never disables the other SSRF guards.
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

// transport wires the SSRF dial guard in via the dialer Control callback, which
// runs on every connection attempt — including redirect targets.
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

// disallowedIP reports whether an IP must not be dialed because it points at a
// private, local, or otherwise non-public destination (SSRF defense). It is the
// pure core of the dial guard so it can be unit-tested without a network.
func disallowedIP(ip netip.Addr) bool {
	ip = ip.Unmap() // treat ::ffff:127.0.0.1 like 127.0.0.1
	switch {
	case ip.IsLoopback(),
		ip.IsPrivate(),          // RFC1918 10/8, 172.16/12, 192.168/16 + ULA fc00::/7
		ip.IsLinkLocalUnicast(), // 169.254.0.0/16 (incl. metadata) + fe80::/10
		ip.IsLinkLocalMulticast(),
		ip.IsMulticast(),
		ip.IsUnspecified(),
		ip.IsInterfaceLocalMulticast():
		return true
	}
	// CGNAT 100.64.0.0/10 is not covered by IsPrivate.
	if ip.Is4() && cgnat.Contains(ip) {
		return true
	}
	return false
}

// cgnat is RFC 6598 shared address space (carrier-grade NAT).
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (linkbuilding.Page, error) {
	// Scheme allowlist before dialing: reject file://, gopher://, ftp://, etc.
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
		// Preserve context cancel/deadline in the chain so callers can errors.Is it.
		return linkbuilding.Page{}, fmt.Errorf("webfetch request %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		f.log.WarnContext(ctx, "webfetch non-2xx", "url", rawURL, "status", resp.StatusCode)
		return linkbuilding.Page{}, fmt.Errorf("webfetch %s: status %d", rawURL, resp.StatusCode)
	}

	// Content-Type gate: don't feed binary/PDF to the HTML parser. Empty type is
	// allowed best-effort (many servers omit it on valid HTML).
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

// truncateRunes caps s at max bytes without splitting a multibyte rune; it
// backs off to the last full rune boundary at or before max.
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Back off while s[max] is a UTF-8 continuation byte, i.e. cut lands mid-rune.
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
