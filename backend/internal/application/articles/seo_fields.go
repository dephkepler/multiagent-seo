package articles

import (
	"regexp"
	"strings"
)

var (
	seoTitleRE = regexp.MustCompile(`(?im)^[\s>*_\-]*\*{0,2}\s*SEO\s+Title\s*\*{0,2}\s*[:\-]\s*(.+?)\s*\*{0,2}\s*$\n?`)
	seoDescRE  = regexp.MustCompile(`(?im)^[\s>*_\-]*\*{0,2}\s*SEO\s+(?:Meta\s+)?Description\s*\*{0,2}\s*[:\-]\s*(.+?)\s*\*{0,2}\s*$\n?`)
)

func extractSEOFields(content string) (cleaned, title, desc string) {
	if m := seoTitleRE.FindStringSubmatch(content); len(m) > 1 {
		title = strings.TrimSpace(stripMarkdown(m[1]))
		content = seoTitleRE.ReplaceAllString(content, "")
	}
	if m := seoDescRE.FindStringSubmatch(content); len(m) > 1 {
		desc = strings.TrimSpace(stripMarkdown(m[1]))
		content = seoDescRE.ReplaceAllString(content, "")
	}
	return strings.TrimSpace(content), title, desc
}

func stripMarkdown(s string) string {
	return strings.Trim(s, " *_`")
}
