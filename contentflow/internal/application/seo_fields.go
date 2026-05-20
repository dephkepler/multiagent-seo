package application

import (
	"regexp"
	"strings"
)

// extractSEOFields pulls "SEO Title:" and "SEO Description:" labelled
// trailers out of the article body. The model emits them per our prompt
// rule, but they're meant for WordPress meta fields, not the published
// article — so we strip them from the body and return them separately.
//
// Matching is permissive: optional bold markdown (**SEO Title:**), case-
// insensitive label, value on the same line.
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

// stripMarkdown removes the basic emphasis markers that wrap the value
// when the LLM bolds it (**Title**, *Title*, _Title_).
func stripMarkdown(s string) string {
	s = strings.Trim(s, " *_`")
	return s
}
