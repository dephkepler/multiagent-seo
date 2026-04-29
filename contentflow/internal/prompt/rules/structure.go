package rules

// Text-layout rules: paragraphs, intro/conclusion shape, tables, lists, images.
var (
	ParagraphFormat = Rule{
		ID:   "structure.paragraph_format",
		Body: "Each paragraph: 4 to 7 sentences, varied lengths",
	}
	ParagraphsAfterHeading = Rule{
		ID:   "structure.paragraphs_after_heading",
		Body: "After a heading there may be 1 to 3 paragraphs of text",
	}
	IntroFormat = Rule{
		ID:   "structure.intro_format",
		Body: "Intro: a few sentences, no heading",
	}
	TablesPlacement = Rule{
		ID:   "structure.tables_placement",
		Body: "1 to 2 tables placed in different parts of the body; not in intro or conclusion; not in adjacent paragraphs",
	}
	ListsCount = Rule{
		ID:   "structure.lists_count",
		Body: "2 to 3 bullet lists and 1 to 2 numbered lists",
	}
	ImagesCount = Rule{
		ID:   "structure.images_count",
		Body: "3 to 4 images (AI-generated, horizontal 1200px) with alt text",
	}
	ImagePlaceholders = Rule{
		ID:   "structure.image_placeholders",
		Body: `Insert image placeholders in format: [IMAGE: alt="keyword description" caption="..."]`,
	}
	FeaturedImagePlaceholder = Rule{
		ID:   "structure.featured_image",
		Body: `Mark one image as the WordPress featured image with alt="<primary keyword>"`,
	}
)

// StructureGroup — paragraph format, intro, tables, lists, images.
func StructureGroup() Group {
	return Group{
		Name: "Structure",
		Rules: []Rule{
			ParagraphFormat,
			ParagraphsAfterHeading,
			IntroFormat,
			TablesPlacement,
			ListsCount,
			ImagesCount,
			ImagePlaceholders,
			FeaturedImagePlaceholder,
		},
	}
}
