package rules

var (
	ImagesCount = Rule{
		ID:   "media.images_count",
		Body: "3-4 images total. Place the first one immediately after the introduction. Distribute the rest evenly through the article",
	}
	ImagePlaceholders = Rule{
		ID:   "media.image_placeholders",
		Body: `Image placeholder format: [IMG | description of what should be in the image | ALT: keyword variation | 1200px horizontal AI-generated]`,
	}
	FeaturedImage = Rule{
		ID:   "media.featured_image",
		Body: "The first image is the WordPress featured image",
	}
	InternalLinks = Rule{
		ID:   "media.internal_links",
		Body: "2-3 internal links placed in the middle section of the article",
	}
	InternalLinkFormat = Rule{
		ID:   "media.internal_link_format",
		Body: `Internal link format: [INTERNAL_LINK | anchor: topical phrase | target page topic]`,
	}
)

func MediaGroup() Group {
	return Group{
		Name: "Media & Internal Links",
		Rules: []Rule{
			ImagesCount,
			ImagePlaceholders,
			FeaturedImage,
			InternalLinks,
			InternalLinkFormat,
		},
	}
}
