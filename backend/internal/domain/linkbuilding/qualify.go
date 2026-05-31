package linkbuilding

import (
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// CountExternalDomains returns how many distinct registrable domains (eTLD+1)
// the page links out to, excluding the site's own domain. Relative, anchor,
// mailto/tel/javascript and unparseable hrefs have no host and are skipped, so
// only genuine cross-domain links count. Subdomains of the site (eTLD+1 match)
// are treated as internal.
func CountExternalDomains(siteURL string, links []string) int {
	own := registrableDomain(siteURL)
	seen := make(map[string]struct{})
	for _, href := range links {
		d := registrableDomain(href)
		if d == "" || d == own {
			continue
		}
		seen[d] = struct{}{}
	}
	return len(seen)
}

// registrableDomain extracts the eTLD+1 (e.g. "example.co.uk") from a URL's
// host, lower-cased. Returns "" when the URL has no parseable host.
func registrableDomain(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return ""
	}
	d, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return ""
	}
	return d
}

// IsSuitable reports whether the classified topic is in the accepted set
// (case-insensitive). An empty topic or empty accepted set is never suitable.
func IsSuitable(topic string, accepted []string) bool {
	t := strings.ToLower(strings.TrimSpace(topic))
	if t == "" {
		return false
	}
	for _, a := range accepted {
		if strings.ToLower(strings.TrimSpace(a)) == t {
			return true
		}
	}
	return false
}
