package rules

var (
	ImagesCount = Rule{
		ID:   "media.images_count",
		Body: "3-4 images total. Place the first one immediately after the introduction. Distribute the rest evenly through the article",
	}
	ImagePlaceholders = Rule{
		ID:   "media.image_placeholders",
		Body: `Image placeholder format: [IMG | description | ALT: keyword variation | 1200px horizontal AI-generated]

The description is fed verbatim to Pexels search, so it MUST be specific and topic-grounded. Follow ALL rules below:

1. Keyword anchor: the description MUST contain the article's primary keyword or a narrow variant of it (e.g. for "game art outsourcing services" use "game artist", "3D character art", "game environment art" — NOT just "artist" or "art").

2. Concrete subject anchor: the description MUST name at least one concrete subject — a person with a role (e.g. "game artist", "unity developer"), a specific tool/device (e.g. "Wacom tablet", "Unreal Engine viewport"), an environment (e.g. "game studio workstation"), AND/OR a concrete action verb (e.g. "sculpting", "rigging", "compositing"). Generic adjectives without a subject are forbidden.

3. Banned generic stock-photo clichés — these literal phrases MUST NOT appear in any description: "business meeting", "team in office", "person at computer", "professional working", "diverse team", "shaking hands", "happy customer". Also avoid vague fillers like "working in studio", "computer with code", "office workspace" on their own.

4. ALT text: 2-4 words, a keyword variant suitable for SEO, NOT a sentence and NOT a duplicate of the description. Examples: "3d character design", "mobile game development", "unity engine workflow".

GOOD: [IMG | game artist designing 3D character on a Wacom tablet, dual monitors | ALT: 3d character design | 1200px horizontal AI-generated]
GOOD: [IMG | unity developer debugging mobile game build on laptop with Android phone tethered | ALT: mobile game development | 1200px horizontal AI-generated]
BAD:  [IMG | artist working in studio | ALT: art | 1200px horizontal AI-generated]
BAD:  [IMG | computer with code | ALT: programming | 1200px horizontal AI-generated]
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
