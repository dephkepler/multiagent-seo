package rules

// DefaultSEO is the baseline preset used by brief/writer/editor prompts.
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
