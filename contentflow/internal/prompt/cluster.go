package prompt

import (
	"fmt"
	"strings"
)

// Cluster holds the target keywords the article must cover and an optional
// suggested title. Rendered as a labeled block appended after the rule list.
type Cluster struct {
	Keywords []string
	Title    string
}

// Render returns the "## Target Keywords" block (with an optional
// "Suggested title" line). Returns an empty string when both fields are empty.
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
		fmt.Fprintf(&b, "Suggested article title (adjust only if clearly better): %s\n", title)
	}
	if len(kws) > 0 {
		b.WriteString("Weave these keywords naturally into the article, following the Keywords rules above:\n")
		for _, k := range kws {
			fmt.Fprintf(&b, "- %s\n", k)
		}
	}
	return b.String()
}
