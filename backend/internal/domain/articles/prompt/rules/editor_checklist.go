package rules

func EditorChecklist() Preset {
	return Preset{
		Name: "editor-checklist",
		Groups: []Group{
			{
				Name: "Editor checklist",
				Rules: []Rule{
					ExactMatchPositions,
					KeywordDensity,
					KeywordMaxPerParagraph,
					PassiveVoiceLimit,
					BannedPhrases,
					SEOTitleFormat,
					SEODescriptionFormat,
					ImagesCount,
					InternalLinks,
					ConclusionHeadingNatural,
				},
			},
		},
	}
}
