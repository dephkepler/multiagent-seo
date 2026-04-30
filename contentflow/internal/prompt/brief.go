package prompt

import (
	"fmt"

	"contentflow/internal/prompt/rules"
)

func Brief(keyword, language, siteTopic, extraRules string, competitors Competitors) string {
	return fmt.Sprintf(`You are an SEO content strategist. Create a detailed article brief for the keyword: "%s".
Language: %s.
%s
%s

%s
Return a structured brief with these sections:

1. TITLE (H1)
2. SEO TITLE
3. SEO DESCRIPTION
4. TARGET AUDIENCE
5. SEARCH INTENT
6. ARTICLE STRUCTURE (H2s, H3s, where to place tables and lists)
7. KEY POINTS per section
8. LSI KEYWORDS
9. AUTHORITATIVE SOURCE (one real expert source or statistic)
10. IMAGE SUGGESTIONS (3-4 descriptions, horizontal 1200px, alt text using the keyword)

Follow these rules:

%s`,
		keyword,
		language,
		siteTopicLine(siteTopic),
		extraRulesLine(extraRules),
		competitors.Render(),
		rules.DefaultSEO().Render(),
	)
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
