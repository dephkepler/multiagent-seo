package publisher

import (
	"context"
	"strings"

	"contentflow/internal/pexels"
)

// PexelsResolver searches Pexels using "<keyword> <alt> <description>".
// ALT carries the SEO keyword variant, so weighting it ahead of the
// LLM's free-form description biases results toward the article topic.
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
	photo, err := p.client.Search(ctx, query)
	if err != nil {
		return ResolvedImage{}, err
	}
	return ResolvedImage{
		URL:             photo.URL,
		Photographer:    photo.Photographer,
		PhotographerURL: photo.PhotographerURL,
		SourceURL:       photo.SourceURL,
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
