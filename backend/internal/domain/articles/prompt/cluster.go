// Package prompt builds the LLM prompts for the article-generation pipeline.
package prompt

import (
	"fmt"
	"strings"

	"multiagent-seo/internal/domain/articles"
)

// renderCluster returns the "## Target Keywords" block, or empty when both the
// title and keywords are empty.
func renderCluster(c articles.Cluster) string {
	title := strings.TrimSpace(c.Title)
	var kws []string
	for _, k := range c.Keywords {
		if k = strings.TrimSpace(k); k != "" {
			kws = append(kws, k)
		}
	}
	if title == "" && len(kws) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Target Keywords\n")
	if title != "" {
		fmt.Fprintf(&b, "Article H1 (use exactly this wording, do not rewrite or translate): %s\n", title)
	}
	if len(kws) > 0 {
		b.WriteString("Weave EVERY keyword below naturally into the article body. All of them must appear at least once:\n")
		for _, k := range kws {
			fmt.Fprintf(&b, "- %s\n", k)
		}
	}
	return b.String()
}
