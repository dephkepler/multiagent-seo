package rules

// Heading structure and SEO-meta rules.
var (
	H1Unique = Rule{
		ID:   "titles.h1_unique",
		Body: "Exactly one H1 per article",
	}
	H2Count = Rule{
		ID:   "titles.h2_count",
		Body: "5 to 10 H2 headings in a logical order",
	}
	H3Count = Rule{
		ID:   "titles.h3_count",
		Body: "0 to 5 H3 subheadings total, only where structurally needed",
	}
	HeadingHierarchy = Rule{
		ID:   "titles.heading_hierarchy",
		Body: "Follow a logical hierarchy: H1 → H2 → H3 (H3 only under an H2)",
	}
	NoH3RightAfterH2 = Rule{
		ID:   "titles.no_h3_right_after_h2",
		Body: "H3 must not follow H2 without at least one paragraph of text between them",
	}
	SEOTitleLength = Rule{
		ID:   "titles.seo_title_length",
		Body: "SEO TITLE: max 60 characters, must contain the primary keyword",
	}
	SEODescriptionLength = Rule{
		ID:   "titles.seo_description_length",
		Body: "SEO DESCRIPTION: max 155 characters, must contain the primary keyword",
	}
	SEOFieldsPresent = Rule{
		ID:   "titles.seo_fields_present",
		Body: "Output SEO TITLE and SEO DESCRIPTION as separate labeled fields at the end (for WP admin)",
	}
)

// TitlesGroup — heading structure and SEO metadata rules.
func TitlesGroup() Group {
	return Group{
		Name: "Titles & Meta",
		Rules: []Rule{
			H1Unique,
			H2Count,
			H3Count,
			HeadingHierarchy,
			NoH3RightAfterH2,
			SEOTitleLength,
			SEODescriptionLength,
			SEOFieldsPresent,
		},
	}
}
