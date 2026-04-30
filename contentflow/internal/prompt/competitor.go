package prompt

import (
	"fmt"
	"strings"
)

// CompetitorItem is a single SERP result passed into prompts.
type CompetitorItem struct {
	Rank        int
	Title       string
	Description string
}

// Competitors holds the SERP analysis context used to enrich briefs.
type Competitors struct {
	Items []CompetitorItem
}

// Render returns a formatted section for inclusion in a prompt.
// Returns an empty string when there are no items.
func (c Competitors) Render() string {
	if len(c.Items) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Top Competitor Pages (SERP)\n")
	sb.WriteString("Study these top-ranking pages to understand what Google rewards for this keyword:\n\n")

	for _, item := range c.Items {
		fmt.Fprintf(&sb, "%d. **%s**\n   %s\n\n", item.Rank, item.Title, item.Description)
	}

	sb.WriteString("Use competitor titles and descriptions to identify gaps and angles they missed. ")
	sb.WriteString("Do NOT copy their content — outperform it.\n")

	return sb.String()
}
