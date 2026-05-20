package publisher

import (
	"context"
	"strings"

	"contentflow/internal/pexels"
)

// pexelsCandidates is how many photos we ask Pexels for per query before
// scoring. 5 hits a sweet spot — enough variety to filter out clearly
// off-topic shots, low enough to stay inside the free tier's 200/hour cap.
const pexelsCandidates = 5

// PexelsResolver searches Pexels using "<keyword> <alt> <description>",
// asks for several candidates, and picks the one whose Pexels-provided
// alt text actually shares tokens with the article keyword. Without that
// filter Pexels happily returns whatever ranks for the literal query,
// which often drifts into abstract / metaphor territory ("creativity",
// "design canvas") for niche topics.
type PexelsResolver struct {
	client *pexels.Client
}

// NewPexelsResolver returns nil for a nil client so the result can be
// passed straight into RenderHTML (which treats nil as "strip").
func NewPexelsResolver(client *pexels.Client) ImageResolver {
	if client == nil {
		return nil
	}
	return &PexelsResolver{client: client}
}

func (p *PexelsResolver) Resolve(ctx context.Context, keyword, description, alt string) (ResolvedImage, error) {
	query := buildQuery(keyword, description, alt)
	if query == "" {
		return ResolvedImage{}, nil
	}

	photos, err := p.client.SearchN(ctx, query, pexelsCandidates)
	if err != nil {
		return ResolvedImage{}, err
	}
	if len(photos) == 0 {
		return ResolvedImage{}, nil
	}

	picked := pickRelevant(photos, keyword, alt)
	if picked == nil {
		// All candidates failed the topical filter — skip rather than insert
		// an unrelated photo. RenderHTML strips the placeholder.
		return ResolvedImage{}, nil
	}
	return ResolvedImage{
		URL:             picked.URL,
		Photographer:    picked.Photographer,
		PhotographerURL: picked.PhotographerURL,
		SourceURL:       picked.SourceURL,
	}, nil
}

func buildQuery(keyword, description, alt string) string {
	parts := make([]string, 0, 3)
	for _, s := range []string{keyword, alt, description} {
		if t := strings.TrimSpace(s); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

// pickRelevant returns the first candidate whose alt + page slug contains
// at least one keyword token, or nil if no candidate matches. Keyword and
// the placeholder ALT contribute tokens — the placeholder ALT is the LLM's
// short SEO variant of the keyword, so it adds useful synonyms.
func pickRelevant(photos []pexels.Photo, keyword, placeholderALT string) *pexels.Photo {
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

// stopWords is a tiny stop-list — generic words that would over-match
// against any landscape stock photo and dilute the relevance signal.
var stopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "into": {},
	"this": {}, "that": {}, "your": {}, "you": {}, "are": {}, "was": {},
	"how": {}, "what": {}, "why": {}, "when": {}, "where": {},
	"services": {}, "service": {}, "company": {}, "solution": {}, "solutions": {},
	"best": {}, "top": {}, "guide": {}, "review": {},
}

// tokenize lowercases, splits on non-letters, drops short tokens and
// stop-words, and returns a set so repeated tokens don't double-count.
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

// slugWords pulls human-readable words out of a Pexels page URL like
// https://www.pexels.com/photo/woman-using-laptop-12345/ — these often
// describe the photo better than the alt field on its own.
func slugWords(sourceURL string) string {
	// Replace path separators and dashes with spaces; tokenize handles the rest.
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
