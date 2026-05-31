package rules

var (
	H1Unique = Rule{
		ID:   "titles.h1_unique",
		Body: "Exactly one H1, exact match of the primary keyword, up to 60 characters",
	}
	H2Count = Rule{
		ID:   "titles.h2_count",
		Body: "5 to 10 H2 headings; each H2 must include the primary keyword OR an LSI phrase",
	}
	H2VariedFormats = Rule{
		ID:   "titles.h2_varied_formats",
		Body: `Vary H2 formats — questions, statements, "how to + action", numbers`,
	}
	H3Count = Rule{
		ID:   "titles.h3_count",
		Body: "0 to 5 H3 subheadings total, only where a real sub-structure is needed",
	}
	HeadingHierarchy = Rule{
		ID:   "titles.heading_hierarchy",
		Body: "Follow a logical hierarchy: H1 → H2 → H3 (H3 only under an H2)",
	}
	H3AfterH2Paragraph = Rule{
		ID:   "titles.h3_after_h2_paragraph",
		Body: "After every H2 at least one paragraph of text must come before any H3",
	}
	SEOTitleFormat = Rule{
		ID:   "titles.seo_title_format",
		Body: "SEO Title: exact keyword + value/year | brand — max 60 characters",
	}
	SEODescriptionFormat = Rule{
		ID:   "titles.seo_description_format",
		Body: "SEO Description: keyword + what the reader gets + CTA — strictly 150-160 characters",
	}
	SEOFieldsPresent = Rule{
		ID:   "titles.seo_fields_present",
		Body: "Output SEO Title and SEO Description as plain-text labeled fields at the end of the article (for WP admin)",
	}
)

func TitlesGroup() Group {
	return Group{
		Name: "Titles & Meta",
		Rules: []Rule{
			H1Unique,
			H2Count,
			H2VariedFormats,
			H3Count,
			HeadingHierarchy,
			H3AfterH2Paragraph,
			SEOTitleFormat,
			SEODescriptionFormat,
			SEOFieldsPresent,
		},
	}
}
