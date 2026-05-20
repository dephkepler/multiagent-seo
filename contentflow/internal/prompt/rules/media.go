package rules

var (
	ImagesCount = Rule{
		ID:   "media.images_count",
		Body: "3-4 images total. Place the first one immediately after the introduction. Distribute the rest evenly through the article",
	}
	ImagePlaceholders = Rule{
		ID:   "media.image_placeholders",
		Body: `Image placeholder format: [IMG | description | ALT: keyword variation | 1200px horizontal AI-generated]

The description is fed verbatim to a Pexels stock-photo search, so it MUST be a real, photographable scene that matches the article topic. Treat it as a Google Images search query. Follow ALL rules below:

1. MANDATORY first word: every description MUST start with the article's primary keyword (or its narrow variant) verbatim. Example: for the article "indie game concept art services" every description starts with "indie game concept art" or a narrow variant like "game concept artist", "concept art studio".

2. Concrete subject anchor: after the keyword, the description MUST name at least one concrete photographable subject — a person with a role (e.g. "concept artist", "unity developer"), a specific tool/device (e.g. "Wacom tablet", "Unreal Engine viewport", "Cintiq display"), an environment (e.g. "game studio workstation"), AND/OR a concrete action verb (e.g. "sketching", "sculpting", "rigging", "compositing"). Generic adjectives without a subject are forbidden.

3. NEVER use abstract or metaphorical descriptions. Forbidden: "creative inspiration", "design canvas", "ideas flowing", "vision and creativity", anything that isn't a literal photographable scene. If you cannot describe what a photographer would frame in the viewfinder, the description is wrong.

4. Banned generic stock-photo clichés — these literal phrases MUST NOT appear in any description: "business meeting", "team in office", "person at computer", "professional working", "diverse team", "shaking hands", "happy customer", "working in studio", "computer with code", "office workspace".

5. ALT text: 2-4 words, a keyword variant suitable for SEO, NOT a sentence and NOT a duplicate of the description. Examples: "3d character design", "mobile game development", "unity engine workflow".

GOOD: [IMG | indie game concept art studio with two artists reviewing character sketches on a Cintiq | ALT: indie game concept art | 1200px horizontal AI-generated]
GOOD: [IMG | game concept artist sketching a fantasy creature on a Wacom tablet, dual monitors | ALT: concept art services | 1200px horizontal AI-generated]
GOOD: [IMG | unity developer debugging mobile game build on laptop with Android phone tethered | ALT: mobile game development | 1200px horizontal AI-generated]
BAD:  [IMG | creative inspiration on a blank canvas | ALT: creativity | 1200px horizontal AI-generated]
BAD:  [IMG | artist working in studio | ALT: art | 1200px horizontal AI-generated]
BAD:  [IMG | diverse team in office | ALT: teamwork | 1200px horizontal AI-generated]`,
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
