package rules

// EditorChecklist is a slim preset for the Editor pass that reuses rule bodies
// from other files to keep the prompt token-cheap.
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
