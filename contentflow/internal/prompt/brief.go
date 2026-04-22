package prompt

import "fmt"

func Brief(keyword, language, siteTopic, extraRules string) string {
	return fmt.Sprintf(`You are an SEO content strategist. Create a detailed article brief for the keyword: "%s".
Language: %s.
%s
%s

Return a structured brief with the following sections:

1. TITLE (H1) — must contain the exact keyword
2. SEO TITLE — for WordPress, max 60 characters, contains keyword
3. SEO DESCRIPTION — for WordPress, max 155 characters, contains keyword
4. TARGET AUDIENCE — who is this article for
5. SEARCH INTENT — what the user wants to find
6. ARTICLE STRUCTURE:
   - H2 headings (5 to 10), logical order
   - H3 subheadings where needed (0 to 5 total), never right after H2 without text
   - Mark where to place 1-2 tables (not in intro or conclusion, not next to each other)
   - Mark where to place 2-3 bullet lists and 1-2 numbered lists
7. KEY POINTS — main ideas to cover in each section
8. LSI KEYWORDS — 10-15 semantically related words and phrases to use naturally
9. AUTHORITATIVE SOURCE — one real expert source or statistic to reference
10. IMAGE SUGGESTIONS — 3-4 image descriptions for AI generation (horizontal 1200px), each with alt text using the keyword

Rules:
- Keep it practical and specific, no fluff
- The article style must be conversational, simple language
- Apply EEAT principles (Experience, Expertise, Authoritativeness, Trustworthiness)
- Conclusion heading must NOT use: "conclusion", "final words", "final thoughts", "final verdict" — use a natural heading instead`,
		keyword,
		language,
		siteTopicLine(siteTopic),
		extraRulesLine(extraRules),
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
