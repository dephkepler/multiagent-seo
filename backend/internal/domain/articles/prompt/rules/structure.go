package rules

var (
	ArticleLength = Rule{
		ID:   "structure.article_length",
		Body: "Length: 1400-1900 words (up to 2100 acceptable). Never write less than needed — 1900 is better than 1400",
	}
	ParagraphFormat = Rule{
		ID:   "structure.paragraph_format",
		Body: "Each paragraph: 4 to 7 sentences with varied lengths (mix short and long)",
	}
	MaxSentenceLength = Rule{
		ID:   "structure.max_sentence_length",
		Body: "Maximum sentence length: 30 words",
	}
	ParagraphsAfterHeading = Rule{
		ID:   "structure.paragraphs_after_heading",
		Body: "After every heading: 1 to 3 paragraphs of text",
	}
	IntroFormat = Rule{
		ID:   "structure.intro_format",
		Body: "Intro: 3-5 sentences, no heading. Open with a fact, number, or direct answer to the main question. The primary keyword must appear within the first 100 words",
	}
	IntroForbiddenOpeners = Rule{
		ID:   "structure.intro_forbidden_openers",
		Body: `Intro must NOT start with "In this article...", "Today we'll cover...", "Everyone knows..." or similar generic openers`,
	}
	ConclusionFormat = Rule{
		ID:   "structure.conclusion_format",
		Body: "Conclusion: 4-6 sentences with a clear takeaway plus a call to action",
	}
	ConclusionHeadingNatural = Rule{
		ID:   "structure.conclusion_heading_natural",
		Body: `Conclusion heading: a natural thematic phrasing ("Who Should Choose X", "Which Option Is Right for You") — NEVER "Conclusion", "Final Verdict", "Final Thoughts", "Wrapping Up"`,
	}
	TablesPlacement = Rule{
		ID:   "structure.tables_placement",
		Body: "1-2 tables total. Not in the first or last paragraph. Never in two consecutive sections back to back. Minimum 3 data rows plus a header row. Use concrete data: numbers, comparisons, conditions, deadlines",
	}
	BulletListsCount = Rule{
		ID:   "structure.bullet_lists_count",
		Body: "2-3 bullet lists, minimum 4 items each",
	}
	NumberedListsCount = Rule{
		ID:   "structure.numbered_lists_count",
		Body: "1-2 numbered lists, only for step-by-step instructions or rankings",
	}
)

func StructureGroup() Group {
	return Group{
		Name: "Structure",
		Rules: []Rule{
			ArticleLength,
			ParagraphFormat,
			MaxSentenceLength,
			ParagraphsAfterHeading,
			IntroFormat,
			IntroForbiddenOpeners,
			ConclusionFormat,
			ConclusionHeadingNatural,
			TablesPlacement,
			BulletListsCount,
			NumberedListsCount,
		},
	}
}
