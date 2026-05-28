package prompt

import (
	"fmt"
	"net/url"
	"strings"

	"multiagent-seo/internal/domain/generate"
)

type CompetitorItem struct {
	Rank        int
	URL         string
	Title       string
	Description string
}

type FeaturedSnippetItem struct {
	Title       string
	Description string
}

type Competitors struct {
	Items           []CompetitorItem
	PAA             []string
	FeaturedSnippet *FeaturedSnippetItem
}

// CompetitorsFrom adapts the SERP port's CompetitorData into the prompt's
// render shape. A nil data yields an empty (no-op) Competitors.
func CompetitorsFrom(data *generate.CompetitorData) Competitors {
	var c Competitors
	if data == nil {
		return c
	}
	for _, r := range data.Results {
		c.Items = append(c.Items, CompetitorItem{
			Rank:        r.Rank,
			URL:         r.URL,
			Title:       r.Title,
			Description: r.Description,
		})
	}
	c.PAA = append(c.PAA, data.PAA...)
	if data.FeaturedSnippet != nil {
		c.FeaturedSnippet = &FeaturedSnippetItem{
			Title:       data.FeaturedSnippet.Title,
			Description: data.FeaturedSnippet.Description,
		}
	}
	return c
}

// Render returns a formatted SERP-context section, or empty when nothing is set.
func (c Competitors) Render() string {
	if len(c.Items) == 0 && len(c.PAA) == 0 && c.FeaturedSnippet == nil {
		return ""
	}

	var sb strings.Builder

	if len(c.Items) > 0 {
		sb.WriteString("## Top Competitor Pages (SERP)\n")
		sb.WriteString("Study these top-ranking pages to understand what Google rewards for this keyword:\n\n")

		for _, item := range c.Items {
			host := hostOf(item.URL)
			if host != "" {
				fmt.Fprintf(&sb, "%d. **%s** (%s)\n   %s\n\n", item.Rank, item.Title, host, item.Description)
			} else {
				fmt.Fprintf(&sb, "%d. **%s**\n   %s\n\n", item.Rank, item.Title, item.Description)
			}
		}

		sb.WriteString("Use competitor titles and descriptions to identify gaps and angles they missed. ")
		sb.WriteString("Do NOT copy their content — outperform it.\n")
	}

	if len(c.PAA) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## People Also Ask\n")
		for _, q := range c.PAA {
			fmt.Fprintf(&sb, "- %s\n", q)
		}
		sb.WriteString("\nUse these as cues for H2 sub-topics and LSI coverage.\n")
	}

	if c.FeaturedSnippet != nil {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Featured Snippet target (Google position 0)\n")
		fmt.Fprintf(&sb, "Title: %s\n", c.FeaturedSnippet.Title)
		fmt.Fprintf(&sb, "Description: %s\n", c.FeaturedSnippet.Description)
		sb.WriteString("\nBeat this with a more specific, concrete answer in your intro or first H2.\n")
	}

	return sb.String()
}

// RenderSlim returns a compact SERP context block for the writer/editor, where
// the full brief is also present and we only need a quick competitor recap.
func (c Competitors) RenderSlim() string {
	if len(c.Items) == 0 && len(c.PAA) == 0 && c.FeaturedSnippet == nil {
		return ""
	}

	var sb strings.Builder

	if len(c.Items) > 0 {
		sb.WriteString("## SERP context — top competitors\n")
		for _, item := range c.Items {
			host := hostOf(item.URL)
			if host != "" {
				fmt.Fprintf(&sb, "%d. %s (%s)\n", item.Rank, item.Title, host)
			} else {
				fmt.Fprintf(&sb, "%d. %s\n", item.Rank, item.Title)
			}
		}
	}

	if len(c.PAA) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## People Also Ask\n")
		for _, q := range c.PAA {
			fmt.Fprintf(&sb, "- %s\n", q)
		}
	}

	if c.FeaturedSnippet != nil {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Featured Snippet to beat\n")
		fmt.Fprintf(&sb, "%s — %s\n", c.FeaturedSnippet.Title, c.FeaturedSnippet.Description)
	}

	return sb.String()
}

func hostOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
}
