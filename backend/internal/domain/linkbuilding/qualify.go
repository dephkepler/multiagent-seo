package linkbuilding

import (
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

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
