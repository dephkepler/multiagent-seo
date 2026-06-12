package rules

var (
	Conversational = Rule{
		ID:   "style.conversational",
		Body: `Active voice and direct statements. Avoid bureaucratic constructs like "It is believed that..." or "One could argue that..." — write as if explaining to a colleague`,
	}
	LeadWithConclusion = Rule{
		ID:   "style.lead_with_conclusion",
		Body: "Lead with the conclusion in each section, then explain — not the other way around",
	}
	NoWater = Rule{
		ID:   "style.no_water",
		Body: "No filler. Do not repeat the same idea in different words — every paragraph must carry new information",
	}
	PassiveVoiceLimit = Rule{
		ID:   "style.passive_voice_limit",
		Body: "Passive voice: no more than 15% of sentences",
	}
	VariedSentenceTypes = Rule{
		ID:   "style.varied_sentence_types",
		Body: `Vary sentence types throughout — statements, concessions ("yes, this works, but only if..."), rhetorical questions, personal assessments ("honestly", "in practice")`,
	}
	SpecificsOverGeneralizations = Rule{
		ID:   "style.specifics_over_generalizations",
		Body: `Use specifics instead of vague generalizations — not "many experts say" but an actual source or number`,
	}
	BannedPhrases = Rule{
		ID:   "style.banned_phrases",
		Body: `Banned words and phrases: unique, comprehensive, multifaceted, "it should be noted", "it is worth mentioning", thus, aforementioned, "is" (replace with a direct verb), "currently" (replace with a specific year or fact), leverages, utilises, "in today's world", "in conclusion it can be said"`,
	}
	NumbersAsDigits = Rule{
		ID:   "style.numbers_as_digits",
		Body: `Write numbers and percentages as digits — "67%" not "sixty-seven percent", "5" not "five"`,
	}
)

func StyleGroup() Group {
	return Group{
		Name: "Writing Style",
		Rules: []Rule{
			Conversational,
			LeadWithConclusion,
			NoWater,
			PassiveVoiceLimit,
			VariedSentenceTypes,
			SpecificsOverGeneralizations,
			BannedPhrases,
			NumbersAsDigits,
		},
	}
}
