package rules

// Style, tone, trust signals, and naming conventions.
var (
	Conversational = Rule{
		ID:   "style.conversational",
		Body: "Style: conversational, simple language, no complex literary or academic constructions",
	}
	NoWater = Rule{
		ID:   "style.no_water",
		Body: "No filler text — every paragraph must carry information",
	}
	NoAIFillerPhrases = Rule{
		ID:   "style.no_ai_filler",
		Body: `No AI filler phrases ("In conclusion", "It is worth noting", "It goes without saying", etc.)`,
	}
	ConclusionHeadingNatural = Rule{
		ID:   "style.conclusion_heading_natural",
		Body: `Conclusion heading must NOT be "Conclusion", "Final words", "Final thoughts", or "Final verdict" — use a natural conversational heading`,
	}
	EEAT = Rule{
		ID:   "style.eeat",
		Body: "Apply EEAT (Experience, Expertise, Authoritativeness, Trust) signals throughout the article",
	}
	AuthoritativeSource = Rule{
		ID:   "style.authoritative_source",
		Body: `Reference at least one authoritative source or expert opinion — a nofollow link is acceptable ([SOURCE: "..." url="..."])`,
	}
	PracticalNoFluff = Rule{
		ID:   "style.practical_no_fluff",
		Body: "Keep it practical and specific, no fluff",
	}
)

// StyleGroup — tone, anti-clichés, EEAT, conclusion heading naming.
func StyleGroup() Group {
	return Group{
		Name: "Style & Quality",
		Rules: []Rule{
			Conversational,
			NoWater,
			NoAIFillerPhrases,
			ConclusionHeadingNatural,
			EEAT,
			AuthoritativeSource,
			PracticalNoFluff,
		},
	}
}
