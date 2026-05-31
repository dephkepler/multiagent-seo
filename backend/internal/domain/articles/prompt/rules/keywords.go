package rules

var (
	ExactMatchPositions = Rule{
		ID:   "keywords.exact_match_positions",
		Body: "Exact-match primary keyword required in: SEO Title, H1, the first 100 words of the intro, at least one H2, and the conclusion",
	}
	IndirectMatches = Rule{
		ID:   "keywords.indirect_matches",
		Body: "Indirect matches (variations, morphological word forms) spread across at least 4 sections: intro, two different mid-body H2s, and the conclusion",
	}
	KeywordDensity = Rule{
		ID:   "keywords.density",
		Body: "Overall keyword density (primary + variations): 2.5-3% of total text",
	}
	KeywordMaxPerParagraph = Rule{
		ID:   "keywords.max_per_paragraph",
		Body: "Max 2 keyword mentions in a single paragraph",
	}
	LSICoverage = Rule{
		ID:   "keywords.lsi_coverage",
		Body: "Use the majority of phrases from the semantic cluster (target: 70%+ coverage). Insert them naturally in context and spread them evenly across the article — do not cluster them in one section",
	}
	LSICountInBrief = Rule{
		ID:   "keywords.lsi_count_in_brief",
		Body: "Brief must suggest 10-15 LSI keywords (semantically related words and phrases)",
	}
	TargetKeywordsAllUsed = Rule{
		ID:   "keywords.target_all_used",
		Body: "Every target keyword from the TARGET KEYWORDS block must appear in the article at least once, in natural context",
	}
	TargetKeywordsNaturalSpread = Rule{
		ID:   "keywords.target_natural_spread",
		Body: "Spread target keywords across different sections — do not cluster them in one paragraph",
	}
)

func KeywordsGroup() Group {
	return Group{
		Name: "Keywords",
		Rules: []Rule{
			ExactMatchPositions,
			IndirectMatches,
			KeywordDensity,
			KeywordMaxPerParagraph,
			LSICoverage,
			LSICountInBrief,
			TargetKeywordsAllUsed,
			TargetKeywordsNaturalSpread,
		},
	}
}
