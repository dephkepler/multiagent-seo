package prompt

import "fmt"

func Editor(article, keyword string, minWords, maxWords int) string {
	return fmt.Sprintf(`You are an SEO editor. Review and improve the article below without changing its meaning or structure.

KEYWORD: "%s"
REQUIRED WORD COUNT: %d to %d words

ARTICLE:
%s

EDITING CHECKLIST — fix everything that does not comply:
1. Word count is within %d-%d words — if shorter, expand paragraphs naturally
2. Keyword appears in H1 — add if missing
3. Keyword density is 2.5-3%% — adjust if too high or too low
4. Max 2 keyword repetitions per paragraph — reduce if exceeded
5. Each paragraph has 4-7 sentences with varied lengths — fix short or long paragraphs
6. No H3 immediately after H2 without text between them — add a sentence if needed
7. LSI keywords are used naturally — weave in any missing ones
8. Conclusion heading is NOT an AI cliché — rename if it is
9. No AI filler phrases ("In conclusion", "It is worth noting", "It goes without saying") — rewrite them
10. Tables are not in intro or conclusion and not placed next to each other — move if needed
11. EEAT signals present (expert source, experience, trust) — add if missing
12. SEO TITLE and SEO DESCRIPTION are present at the end — add if missing

Return the fully corrected article only. No explanations.`,
		keyword,
		minWords,
		maxWords,
		article,
		minWords,
		maxWords,
	)
}
