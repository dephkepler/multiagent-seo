package articles

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

var (
	imgPlaceholderRE          = regexp.MustCompile(`\[IMG\s*\|[^\]]*?\]`)
	internalLinkPlaceholderRE = regexp.MustCompile(`\[INTERNAL_LINK\s*\|[^\]]*?\]`)
)

type RenderOptions struct {
	Keyword     string
	Resolver    ImageResolver
	Attribution bool
	Log         *slog.Logger
}

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

func RenderHTML(ctx context.Context, content string, opts RenderOptions) (string, RenderStats, error) {
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
			if err != nil {
				if opts.Log != nil {
					opts.Log.DebugContext(ctx, "image resolution failed, skipping placeholder", "keyword", opts.Keyword, "desc", desc, "err", err)
				}
				stats.ImagesFailed++
				return ""
			}
			if img.URL == "" {
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
		return "", stats, fmt.Errorf("render markdown: %w", err)
	}
	return buf.String(), stats, nil
}

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

type Photo struct {
	URL             string
	Photographer    string
	PhotographerURL string
	SourceURL       string
	Alt             string
}

func PickRelevant(photos []Photo, keyword, placeholderALT string) *Photo {
	if len(photos) == 0 {
		return nil
	}
	wanted := tokenize(keyword + " " + placeholderALT)
	if len(wanted) == 0 {
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

var stopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "into": {},
	"this": {}, "that": {}, "your": {}, "you": {}, "are": {}, "was": {},
	"how": {}, "what": {}, "why": {}, "when": {}, "where": {},
	"services": {}, "service": {}, "company": {}, "solution": {}, "solutions": {},
	"best": {}, "top": {}, "guide": {}, "review": {},
}

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
