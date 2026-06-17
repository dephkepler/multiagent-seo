package emailscrape

import (
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

// Matches plain-text addresses and the address inside a mailto: href alike.
var emailRe = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// Cloudflare email-protection stores the address hex-encoded in this attribute.
var cfEmailRe = regexp.MustCompile(`data-cfemail="([0-9a-fA-F]+)"`)

// Bracketed at/dot obfuscation, e.g. "info [at] firma [dot] de" (spaces stripped).
var (
	atObfRe  = regexp.MustCompile(`(?i)\s*[\[\({]\s*at\s*[\]\)}]\s*`)
	dotObfRe = regexp.MustCompile(`(?i)\s*[\[\({]\s*dot\s*[\]\)}]\s*`)
)

var imageExt = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".bmp"}

var junkDomains = []string{
	"example.com", "example.org", "example.net", "domain.com", "yourdomain.com",
	"email.com", "test.com", "sentry.io", "wix.com", "wixpress.com", "godaddy.com",
	"schema.org", "w3.org", "sentry-next.wixpress.com",
}

// ExtractEmails pulls unique, normalized, real-looking addresses out of a page's
// raw HTML: plain text, mailto: links, [at]/[dot] obfuscation, and Cloudflare's
// data-cfemail protection. Junk and duplicates are dropped.
func ExtractEmails(html string) []string {
	seen := make(map[string]struct{})
	add := func(raw string) {
		e := strings.ToLower(strings.Trim(raw, ".-_"))
		if !isJunk(e) {
			seen[e] = struct{}{}
		}
	}

	for _, m := range emailRe.FindAllString(deobfuscate(html), -1) {
		add(m)
	}
	for _, m := range cfEmailRe.FindAllStringSubmatch(html, -1) {
		if e := decodeCFEmail(m[1]); e != "" {
			add(e)
		}
	}

	out := make([]string, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

func deobfuscate(s string) string {
	s = atObfRe.ReplaceAllString(s, "@")
	s = dotObfRe.ReplaceAllString(s, ".")
	s = strings.ReplaceAll(s, "&#64;", "@")
	s = strings.ReplaceAll(s, "&#46;", ".")
	return s
}

// decodeCFEmail reverses Cloudflare's XOR scheme: first byte is the key, the
// rest are the address bytes XORed with it.
func decodeCFEmail(hexStr string) string {
	b, err := hex.DecodeString(hexStr)
	if err != nil || len(b) < 2 {
		return ""
	}
	key := b[0]
	out := make([]byte, len(b)-1)
	for i := 1; i < len(b); i++ {
		out[i-1] = b[i] ^ key
	}
	return string(out)
}

func isJunk(email string) bool {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return true
	}
	domain := email[at+1:]
	for _, ext := range imageExt {
		if strings.HasSuffix(email, ext) {
			return true
		}
	}
	for _, d := range junkDomains {
		if domain == d || strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	if strings.Contains(domain, "wixpress") || strings.Contains(domain, "sentry") {
		return true
	}
	return false
}
