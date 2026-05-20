package rules

// E-E-A-T rules: Experience, Expertise, Authoritativeness, Trustworthiness.
var (
	Experience = Rule{
		ID:   "eeat.experience",
		Body: `Experience: at least 2 concrete real-world examples in the article (e.g. "In testing, this reduced load time by 40%", "After a month of daily use, the pattern emerged")`,
	}
	Expertise = Rule{
		ID:   "eeat.expertise",
		Body: `Expertise: include a number or statistic in every H2 section. Explain not just "what" but "why" and "how"`,
	}
	Authoritativeness = Rule{
		ID:   "eeat.authoritativeness",
		Body: `Authoritativeness: 1-2 links to authoritative sources (regulator, official website, study) using exact format <a href="URL" rel="nofollow" target="_blank">anchor text</a>`,
	}
	Trustworthiness = Rule{
		ID:   "eeat.trustworthiness",
		Body: `Trustworthiness: for every major claim, state one specific limitation or condition ("works best when...", "does not apply if..."). Minimum 2 such caveats per article`,
	}
)

func EEATGroup() Group {
	return Group{
		Name: "E-E-A-T",
		Rules: []Rule{
			Experience,
			Expertise,
			Authoritativeness,
			Trustworthiness,
		},
	}
}
