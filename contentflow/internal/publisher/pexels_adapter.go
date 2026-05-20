package publisher

import (
	"context"
	"strings"

	"contentflow/internal/pexels"
)

// PexelsResolver searches Pexels using the article keyword combined with
// the placeholder description; ALT is only a fallback when description is
// empty since it describes the keyword variation, not the desired image.
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
	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = strings.TrimSpace(alt)
	}
	kw := strings.TrimSpace(keyword)
	switch {
	case kw != "" && desc != "":
		return kw + " " + desc
	case desc != "":
		return desc
	default:
		return kw
	}
}
