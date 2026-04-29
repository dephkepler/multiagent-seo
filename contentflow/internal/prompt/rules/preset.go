package rules

// DefaultSEO is the baseline preset used by brief/writer/editor prompts.
// Per-request overrides (e.g. Without("structure.tables_placement")) compose
// on top of this.
func DefaultSEO() Preset {
	return Preset{
		Name: "default-seo",
		Groups: []Group{
			TitlesGroup(),
			StructureGroup(),
			KeywordsGroup(),
			StyleGroup(),
		},
	}
}
