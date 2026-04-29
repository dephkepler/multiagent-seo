package rules

// Keyword placement and density rules.
var (
	PrimaryKeywordInTitle = Rule{
		ID:   "keywords.primary_in_title",
		Body: "The primary keyword must appear in the SEO title (direct match)",
	}
	PrimaryKeywordInH1 = Rule{
		ID:   "keywords.primary_in_h1",
		Body: "The primary keyword must appear in the H1 (direct match)",
	}
	KeywordDensity = Rule{
		ID:   "keywords.density",
		Body: "Overall keyword density (primary + variations): 2.5-3% of total text",
	}
	KeywordMaxPerParagraph = Rule{
		ID:   "keywords.max_per_paragraph",
		Body: "Max 2 keyword repetitions per paragraph",
	}
	KeywordVariations = Rule{
		ID:   "keywords.variations",
		Body: "Use both direct and indirect (morphological) keyword variations throughout the text",
	}
	LSIFromBrief = Rule{
		ID:   "keywords.lsi_from_brief",
		Body: "Use all LSI keywords and phrases from the brief naturally in context — this is mandatory",
	}
	LSICountInBrief = Rule{
		ID:   "keywords.lsi_count_in_brief",
		Body: "Suggest 10-15 LSI keywords (semantically related words and phrases) in the brief",
	}
	TargetKeywordsAllUsed = Rule{
		ID:   "keywords.target_all_used",
		Body: "Every target keyword from the TARGET KEYWORDS block must appear in the article at least once, in natural context",
	}
	TargetKeywordsNaturalSpread = Rule{
		ID:   "keywords.target_natural_spread",
		Body: "Spread target keywords across different sections — do not cluster them in one paragraph",
	}
	TargetKeywordInH2 = Rule{
		ID:   "keywords.target_in_h2",
		Body: "At least one target keyword (if provided) must appear in an H2 heading",
	}
)

// KeywordsGroup — placement, density, LSI, and target-keyword handling.
func KeywordsGroup() Group {
	return Group{
		Name: "Keywords",
		Rules: []Rule{
			PrimaryKeywordInTitle,
			PrimaryKeywordInH1,
			KeywordDensity,
			KeywordMaxPerParagraph,
			KeywordVariations,
			LSIFromBrief,
			LSICountInBrief,
			TargetKeywordsAllUsed,
			TargetKeywordsNaturalSpread,
			TargetKeywordInH2,
		},
	}
}
