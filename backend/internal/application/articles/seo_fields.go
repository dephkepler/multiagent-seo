package articles

import (
	"regexp"
	"strings"
)

func extractSEOFields(content string) (cleaned, title, desc string) {
	titleRE := regexp.MustCompile(`(?im)^[\s>*_\-]*\*{0,2}\s*SEO\s+Title\s*\*{0,2}\s*[:\-]\s*(.+?)\s*\*{0,2}\s*$\n?`)
	descRE := regexp.MustCompile(`(?im)^[\s>*_\-]*\*{0,2}\s*SEO\s+(?:Meta\s+)?Description\s*\*{0,2}\s*[:\-]\s*(.+?)\s*\*{0,2}\s*$\n?`)

	if m := titleRE.FindStringSubmatch(content); len(m) > 1 {
		title = strings.TrimSpace(stripMarkdown(m[1]))
		content = titleRE.ReplaceAllString(content, "")
	}
	if m := descRE.FindStringSubmatch(content); len(m) > 1 {
		desc = strings.TrimSpace(stripMarkdown(m[1]))
		content = descRE.ReplaceAllString(content, "")
	}
	return strings.TrimSpace(content), title, desc
}

func stripMarkdown(s string) string {
	return strings.Trim(s, " *_`")
}
