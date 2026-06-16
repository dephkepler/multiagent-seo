package emailscrape

import (
	"regexp"
	"sort"
	"strings"
)

// Matches plain-text addresses and the address inside a mailto: href alike (the
// "mailto:" prefix isn't email-char, so the capture starts at the local part).
var emailRe = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// imageExt catches false positives like "logo@2x.png" — a valid-looking address
// whose TLD is really an image extension.
var imageExt = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".bmp"}

// junkDomains are placeholder/vendor addresses that are never a real contact.
var junkDomains = []string{
	"example.com", "example.org", "example.net", "domain.com", "yourdomain.com",
	"email.com", "test.com", "sentry.io", "wix.com", "wixpress.com", "godaddy.com",
	"schema.org", "w3.org", "sentry-next.wixpress.com",
}

// ExtractEmails pulls unique, normalized, real-looking email addresses out of a
// page's raw HTML (plain text and mailto: links both). Junk and duplicates are dropped.
func ExtractEmails(html string) []string {
	seen := make(map[string]struct{})
	for _, m := range emailRe.FindAllString(html, -1) {
		e := strings.ToLower(strings.Trim(m, ".-_"))
		if isJunk(e) {
			continue
		}
		seen[e] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
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
