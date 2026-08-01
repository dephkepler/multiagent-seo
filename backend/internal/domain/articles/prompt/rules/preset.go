package rules

func DefaultSEO() Preset {
	return Preset{
		Name: "default-seo",
		Groups: []Group{
			TitlesGroup(),
			StructureGroup(),
			StyleGroup(),
			KeywordsGroup(),
			EEATGroup(),
			MediaGroup(),
		},
	}
}
