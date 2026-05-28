package prompt

import (
	"fmt"

	"multiagent-seo/internal/prompt/rules"
)

func Brief(keyword, language string, cluster Cluster, siteTopic, extraRules string, competitors Competitors) string {
	return fmt.Sprintf(`You are an SEO content strategist. Create a detailed article brief for the keyword: "%s".
Language: %s.
%s
%s

%s
%s
Return a structured brief with these sections:

1. TITLE (H1) — exact keyword match, up to 60 characters
2. SEO TITLE — format: exact keyword + value/year | brand, max 60 characters
3. SEO DESCRIPTION — keyword + what the reader gets + CTA, strictly 150-160 characters
4. UNIQUE ANGLE — the perspective or depth this article will take that competitors miss
5. TARGET AUDIENCE
6. SEARCH INTENT
7. ARTICLE STRUCTURE (5-10 H2s in varied formats — question, statement, "how to + action", number; mark where H3s, tables, lists, and images go)
8. KEY POINTS per section — include a concrete number or statistic for every H2
9. LSI KEYWORDS (10-15 phrases for the semantic cluster)
10. AUTHORITATIVE SOURCES — 1-2 real expert sources or studies with URLs (regulator, official website, study)
11. INTERNAL LINK IDEAS — 2-3 suggestions: anchor phrase + target page topic
12. IMAGE SUGGESTIONS — 3-4 descriptions for horizontal 1200px images with alt text using keyword variations; mark the first one as the WordPress featured image

Follow these rules:

%s`,
		keyword,
		language,
		siteTopicLine(siteTopic),
		extraRulesLine(extraRules),
		competitors.Render(),
		clusterBriefInstruction(cluster),
		rules.DefaultSEO().Render(),
	)
}

// clusterBriefInstruction tells the strategist to honour the sheet-supplied H1
// and LSI set instead of inventing new ones that the writer would then have to
// override.
func clusterBriefInstruction(c Cluster) string {
	rendered := c.Render()
	if rendered == "" {
		return ""
	}
	return rendered + "\nUse the H1 above verbatim for sections 1 (TITLE) and as the basis of section 2 (SEO TITLE) — do not invent a new heading. Build section 9 (LSI KEYWORDS) around the target keywords above; you may add a few related phrases but the listed keywords must dominate the set.\n"
}

func siteTopicLine(siteTopic string) string {
	if siteTopic == "" {
		return ""
	}
	return fmt.Sprintf("Site niche: %s.", siteTopic)
}

func extraRulesLine(extraRules string) string {
	if extraRules == "" {
		return ""
	}
	return fmt.Sprintf("Additional rules: %s.", extraRules)
}
