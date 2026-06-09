package postsdiscover

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	userAgent = "Mozilla/5.0 (compatible; multiagent-seo-bot/1.0)"
	maxBody   = 4 << 20
)

type Discoverer struct {
	http *http.Client
	log  *slog.Logger
}

func New(log *slog.Logger) *Discoverer {
	if log == nil {
		log = slog.Default()
	}
	d := &Discoverer{log: log}
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_ string, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return err
			}
			if disallowedIP(ip) {
				return fmt.Errorf("blocked address %s", ip)
			}
			return nil
		},
	}
	d.http = &http.Client{
		Timeout:   12 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext},
	}
	return d
}

func (d *Discoverer) Discover(ctx context.Context, siteURL string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	origin, err := originOf(siteURL)
	if err != nil {
		return nil, err
	}

	if urls := d.fromSitemap(ctx, origin, limit); len(urls) > 0 {
		d.log.InfoContext(ctx, "posts discovered", "site", siteURL, "source", "sitemap", "count", len(urls), "urls", urls)
		return urls, nil
	}
	if urls := d.fromWPRest(ctx, origin, limit); len(urls) > 0 {
		d.log.InfoContext(ctx, "posts discovered", "site", siteURL, "source", "wp-rest", "count", len(urls), "urls", urls)
		return urls, nil
	}
	d.log.InfoContext(ctx, "posts discover empty", "site", siteURL)
	return nil, nil
}

func (d *Discoverer) fromSitemap(ctx context.Context, origin string, limit int) []string {
	for _, path := range []string{"/wp-sitemap.xml", "/sitemap.xml", "/sitemap_index.xml"} {
		urls := d.parseSitemap(ctx, origin+path, limit, 0)
		if len(urls) > 0 {
			return urls
		}
	}
	return nil
}

func (d *Discoverer) parseSitemap(ctx context.Context, smURL string, limit, depth int) []string {
	if depth > 2 {
		return nil
	}
	body, err := d.fetch(ctx, smURL)
	if err != nil {
		return nil
	}

	var idx sitemapIndex
	if err := xml.Unmarshal(body, &idx); err == nil && len(idx.Sitemaps) > 0 {
		sort.Slice(idx.Sitemaps, func(i, j int) bool { return idx.Sitemaps[i].LastMod > idx.Sitemaps[j].LastMod })
		for _, sm := range idx.Sitemaps {
			loc := strings.TrimSpace(sm.Loc)
			if loc == "" {
				continue
			}
			if !looksLikePostsSitemap(loc) {
				continue
			}
			if urls := d.parseSitemap(ctx, loc, limit, depth+1); len(urls) > 0 {
				return urls
			}
		}
		for _, sm := range idx.Sitemaps {
			loc := strings.TrimSpace(sm.Loc)
			if loc == "" {
				continue
			}
			if urls := d.parseSitemap(ctx, loc, limit, depth+1); len(urls) > 0 {
				return urls
			}
		}
		return nil
	}

	var us urlSet
	if err := xml.Unmarshal(body, &us); err != nil || len(us.URLs) == 0 {
		return nil
	}
	sort.Slice(us.URLs, func(i, j int) bool { return us.URLs[i].LastMod > us.URLs[j].LastMod })

	out := make([]string, 0, limit)
	for _, u := range us.URLs {
		loc := strings.TrimSpace(u.Loc)
		if loc == "" || !looksLikePost(loc) {
			continue
		}
		out = append(out, loc)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (d *Discoverer) fromWPRest(ctx context.Context, origin string, limit int) []string {
	u := fmt.Sprintf("%s/wp-json/wp/v2/posts?per_page=%d&orderby=date&order=desc&_fields=link", origin, limit)
	body, err := d.fetch(ctx, u)
	if err != nil {
		return nil
	}
	var arr []struct {
		Link string `json:"link"`
	}
	if err := json.Unmarshal(body, &arr); err != nil {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, p := range arr {
		if p.Link != "" {
			out = append(out, p.Link)
		}
	}
	return out
}

func (d *Discoverer) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

type sitemapIndex struct {
	XMLName  xml.Name `xml:"sitemapindex"`
	Sitemaps []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"sitemap"`
}

type urlSet struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"url"`
}

func originOf(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid url %q", rawURL)
	}
	return u.Scheme + "://" + u.Host, nil
}

func looksLikePostsSitemap(loc string) bool {
	low := strings.ToLower(loc)
	return strings.Contains(low, "post") || strings.Contains(low, "blog") || strings.Contains(low, "article")
}

func looksLikePost(loc string) bool {
	low := strings.ToLower(loc)
	for _, skip := range []string{"/category/", "/tag/", "/author/", "/page/", "/wp-admin/", "/feed", "/comments/"} {
		if strings.Contains(low, skip) {
			return false
		}
	}
	u, err := url.Parse(loc)
	if err != nil {
		return false
	}
	path := strings.TrimSuffix(u.Path, "/")
	if path == "" || path == "/" {
		return false
	}
	return strings.Count(path, "/") >= 1
}

func disallowedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	if ip.Is4() && cgnat.Contains(ip) {
		return true
	}
	return false
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")
