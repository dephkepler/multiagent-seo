package articles

import (
	"regexp"
	"strings"
)

// extractSEOFields pulls "SEO Title:" / "SEO Description:" labelled trailers out
// of the article body: the model emits them per the prompt rule, but they belong
// in WordPress meta, not the published body, so we strip and return them.
// Matching is permissive: optional bold markdown, case-insensitive, same-line value.
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
