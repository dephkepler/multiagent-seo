package prompt

import "fmt"

func Writer(brief, keyword, language string, minWords, maxWords int) string {
	return fmt.Sprintf(`You are an SEO copywriter. Write a full article based on the brief below.

KEYWORD: "%s"
LANGUAGE: %s
WORD COUNT: %d to %d words (strictly follow this range)

BRIEF:
%s

WRITING RULES:
- Include the exact keyword in the H1 title
- Use direct and indirect keyword variations throughout the text
- Max 2 keyword repetitions per paragraph
- Keyword density: 2.5-3%% of total text
- Use all LSI keywords from the brief naturally in context
- Each paragraph: 4 to 7 sentences, vary sentence lengths
- After every H2 there must be at least one paragraph before any H3
- Place tables as specified in the brief (not in intro or conclusion, not adjacent to each other)
- Include 2-3 bullet lists and 1-2 numbered lists
- Intro: 2-4 sentences, no heading
- Conclusion: use a natural conversational heading (NOT: "conclusion", "final words", "final thoughts", "final verdict")
- Style: conversational, simple language, no complex literary constructions, no AI filler phrases
- Apply EEAT: include the authoritative source from the brief as an expert reference
- Add image placeholders in format: [IMAGE: alt="keyword description" caption="..."]
- At the end add SEO TITLE and SEO DESCRIPTION as separate labeled fields

Write the complete article now.`,
		keyword,
		language,
		minWords,
		maxWords,
		brief,
	)
}
