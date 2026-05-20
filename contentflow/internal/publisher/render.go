package publisher

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

// ResolvedImage is what an ImageResolver returns. URL is required; the
// remaining fields drive the attribution <figcaption> Pexels recommends.
type ResolvedImage struct {
	URL             string
	Photographer    string
	PhotographerURL string
	SourceURL       string
}

// ImageResolver turns an [IMG | ...] placeholder into a real image plus
// attribution metadata. Implementations must be safe for concurrent use.
// A Resolve error is non-fatal: callers strip the offending placeholder
// and continue. keyword is the article's target keyword and is included
// so resolvers can build a topical search query.
type ImageResolver interface {
	Resolve(ctx context.Context, keyword, description, alt string) (ResolvedImage, error)
}

// RenderOptions bundles the knobs RenderHTML accepts so callers don't
// have to keep growing a positional argument list.
type RenderOptions struct {
	Keyword     string
	Resolver    ImageResolver
	Attribution bool
}

// RenderStats reports per-render image accounting so callers can persist
// it (e.g. in the article row) and surface it in the API.
type RenderStats struct {
	ImagesRequested int // how many [IMG | ...] placeholders the LLM emitted
	ImagesResolved  int // how many got a real Pexels URL
	ImagesSkipped   int // requested - resolved (no resolver, error, empty URL)
}

// WithUnsafe lets the writer's literal <a rel="nofollow"> EEAT anchors and
// our resolver-substituted <img> tags survive Markdown conversion.
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
// Resolve errors strip just that one so a single Pexels miss doesn't kill
// the article. [INTERNAL_LINK | ...] placeholders are always stripped.
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

// renderFigure wraps the image in <figure>/<figcaption> when attribution
// is on AND we have a photographer name; otherwise emits a bare <img>
// (or <figure><img></figure> without caption) so the markup stays clean.
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
// [IMG | description | ALT: alt text | ...]. Either may be empty when the
// model emits a malformed placeholder.
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
