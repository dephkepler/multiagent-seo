package pexels

import (
	"context"
	"strings"

	"multiagent-seo/internal/domain/generate"
)

// candidates is how many photos we ask Pexels for per query before scoring.
// 5 hits a sweet spot — enough variety to filter out clearly off-topic shots,
// low enough to stay inside the free tier's 200/hour cap.
const candidates = 5

// Resolver implements generate.ImageResolver against the Pexels search API.
// It fetches several candidates per query and delegates topical selection to
// the domain's generate.PickRelevant so the scoring lives in one place.
type Resolver struct {
	client *Client
}

var _ generate.ImageResolver = (*Resolver)(nil)

// New returns a Resolver for the given Pexels API key.
func New(apiKey string) *Resolver {
	return &Resolver{client: newClient(apiKey)}
}

func (r *Resolver) Resolve(ctx context.Context, keyword, description, alt string) (generate.ResolvedImage, error) {
	query := buildQuery(keyword, description, alt)
	if query == "" {
		return generate.ResolvedImage{}, nil
	}

	photos, err := r.client.SearchN(ctx, query, candidates)
	if err != nil {
		return generate.ResolvedImage{}, err
	}
	if len(photos) == 0 {
		return generate.ResolvedImage{}, nil
	}

	// Relevance scoring is domain logic: map adapter photos to generate.Photo
	// and let generate.PickRelevant choose.
	picked := generate.PickRelevant(toDomainPhotos(photos), keyword, alt)
	if picked == nil {
		// All candidates failed the topical filter — skip rather than insert
		// an unrelated photo. RenderHTML strips the placeholder.
		return generate.ResolvedImage{}, nil
	}
	return generate.ResolvedImage{
		URL:             picked.URL,
		Photographer:    picked.Photographer,
		PhotographerURL: picked.PhotographerURL,
		SourceURL:       picked.SourceURL,
	}, nil
}

func toDomainPhotos(photos []Photo) []generate.Photo {
	out := make([]generate.Photo, len(photos))
	for i, p := range photos {
		out[i] = generate.Photo{
			URL:             p.URL,
			Photographer:    p.Photographer,
			PhotographerURL: p.PhotographerURL,
			SourceURL:       p.SourceURL,
			Alt:             p.Alt,
		}
	}
	return out
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
