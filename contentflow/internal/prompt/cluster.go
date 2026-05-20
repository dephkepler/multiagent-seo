package prompt

import (
	"fmt"
	"strings"
)

// Cluster holds the target keywords and H1 heading sourced from the sheet.
type Cluster struct {
	Keywords []string
	Title    string // article H1; when set the LLM must use exact wording
}

// Render returns the "## Target Keywords" block, or empty when both fields are empty.
func (c Cluster) Render() string {
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
