package generate

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// Non-greedy match so consecutive placeholders on one line don't merge.
var (
	imgPlaceholderRE          = regexp.MustCompile(`\[IMG\s*\|[^\]]*?\]`)
	internalLinkPlaceholderRE = regexp.MustCompile(`\[INTERNAL_LINK\s*\|[^\]]*?\]`)
)

// RenderOptions bundles the knobs RenderHTML accepts so callers don't have to
// keep growing a positional argument list.
type RenderOptions struct {
	Keyword     string
	Resolver    ImageResolver
	Attribution bool
}

// WithUnsafe lets the writer's literal <a rel="nofollow"> EEAT anchors and our
// resolver-substituted <img> tags survive Markdown conversion.
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.Strikethrough,
		extension.Linkify,
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		gmhtml.WithUnsafe(),
	),
)

// RenderHTML converts the LLM's Markdown to HTML and reports image-resolve
// stats. A nil Resolver strips all [IMG | ...] placeholders; per-placeholder
// Resolve errors strip just that one so a single image miss doesn't kill the
// article. [INTERNAL_LINK | ...] placeholders are always stripped.
func RenderHTML(ctx context.Context, content string, opts RenderOptions) (string, RenderStats) {
	var stats RenderStats
	var stripped string

	if opts.Resolver == nil {
		stripped = imgPlaceholderRE.ReplaceAllStringFunc(content, func(string) string {
			stats.ImagesRequested++
			stats.ImagesSkipped++
			return ""
		})
	} else {
		stripped = imgPlaceholderRE.ReplaceAllStringFunc(content, func(match string) string {
			stats.ImagesRequested++
			desc, alt := parseImgPlaceholder(match)
			if desc == "" && alt == "" && strings.TrimSpace(opts.Keyword) == "" {
				stats.ImagesSkipped++
				return ""
			}
			img, err := opts.Resolver.Resolve(ctx, opts.Keyword, desc, alt)
			if err != nil || img.URL == "" {
				stats.ImagesSkipped++
				return ""
			}
			stats.ImagesResolved++
			return renderFigure(img, alt, opts.Attribution)
		})
	}
	stripped = internalLinkPlaceholderRE.ReplaceAllString(stripped, "")

	var buf bytes.Buffer
	if err := md.Convert([]byte(stripped), &buf); err != nil {
		return content, stats
	}
	return buf.String(), stats
}

// renderFigure wraps the image in <figure>/<figcaption> when attribution is on
// AND we have a photographer name; otherwise emits a bare <img> so the markup
// stays clean.
func renderFigure(img ResolvedImage, alt string, attribution bool) string {
	imgTag := fmt.Sprintf(`<img src=%q alt=%q loading="lazy" />`,
		html.EscapeString(img.URL),
		html.EscapeString(alt),
	)
	if !attribution || img.Photographer == "" {
		return imgTag
	}

	name := html.EscapeString(img.Photographer)
	var photographerPart string
	if img.PhotographerURL != "" {
		photographerPart = fmt.Sprintf(`<a href=%q rel="nofollow">%s</a>`,
			html.EscapeString(img.PhotographerURL), name)
	} else {
		photographerPart = name
	}

	var pexelsPart string
	if img.SourceURL != "" {
		pexelsPart = fmt.Sprintf(`<a href=%q rel="nofollow">Pexels</a>`,
			html.EscapeString(img.SourceURL))
	} else {
		pexelsPart = "Pexels"
	}

	return fmt.Sprintf("<figure>%s<figcaption>Photo by %s on %s</figcaption></figure>",
		imgTag, photographerPart, pexelsPart)
}

// parseImgPlaceholder extracts description and ALT from
// [IMG | description | ALT: alt text | ...]. Either may be empty when the model
// emits a malformed placeholder.
func parseImgPlaceholder(match string) (desc, alt string) {
	inner := strings.TrimPrefix(match, "[")
	inner = strings.TrimSuffix(inner, "]")
	parts := strings.Split(inner, "|")
	if len(parts) >= 2 {
		desc = strings.TrimSpace(parts[1])
	}
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if strings.HasPrefix(strings.ToUpper(t), "ALT:") {
			alt = strings.TrimSpace(t[len("ALT:"):])
			break
		}
	}
	return desc, alt
}

// Photo is an image-search candidate scored by PickRelevant. Alt and SourceURL
// carry the words the relevance filter matches the article keyword against.
type Photo struct {
	URL             string
	Photographer    string
	PhotographerURL string
	SourceURL       string
	Alt             string
}

// PickRelevant returns the first candidate whose alt + page slug shares at
// least one keyword token, or nil if no candidate matches. Both the article
// keyword and the placeholder ALT (the LLM's short SEO variant) contribute
// tokens. Without this filter image search drifts into abstract/metaphor shots
// for niche topics.
func PickRelevant(photos []Photo, keyword, placeholderALT string) *Photo {
	wanted := tokenize(keyword + " " + placeholderALT)
	if len(wanted) == 0 {
		// No useful keyword tokens to score against — accept the top hit.
		return &photos[0]
	}

	bestIdx, bestScore := -1, 0
	for i, ph := range photos {
		haystack := tokenize(ph.Alt + " " + slugWords(ph.SourceURL))
		score := overlap(wanted, haystack)
		if score > bestScore {
			bestIdx, bestScore = i, score
		}
	}
	if bestIdx < 0 {
		return nil
	}
	return &photos[bestIdx]
}

// stopWords is a tiny stop-list — generic words that would over-match against
// any landscape stock photo and dilute the relevance signal.
var stopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "into": {},
	"this": {}, "that": {}, "your": {}, "you": {}, "are": {}, "was": {},
	"how": {}, "what": {}, "why": {}, "when": {}, "where": {},
	"services": {}, "service": {}, "company": {}, "solution": {}, "solutions": {},
	"best": {}, "top": {}, "guide": {}, "review": {},
}

// tokenize lowercases, splits on non-letters, drops short tokens and stop-words,
// and returns a set so repeated tokens don't double-count.
func tokenize(s string) map[string]struct{} {
	out := map[string]struct{}{}
	cur := strings.Builder{}
	flush := func() {
		t := cur.String()
		cur.Reset()
		if len(t) < 3 {
			return
		}
		if _, skip := stopWords[t]; skip {
			return
		}
		out[t] = struct{}{}
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// slugWords pulls human-readable words out of an image page URL like
// https://www.pexels.com/photo/woman-using-laptop-12345/ — these often describe
// the photo better than the alt field on its own.
func slugWords(sourceURL string) string {
	r := strings.NewReplacer("/", " ", "-", " ", "_", " ", ".", " ")
	return r.Replace(sourceURL)
}

func overlap(a, b map[string]struct{}) int {
	n := 0
	for k := range a {
		if _, ok := b[k]; ok {
			n++
		}
	}
	return n
}
