package pexels

import (
	"context"
	"log/slog"
	"strings"

	"multiagent-seo/internal/domain/articles"
)

const candidates = 5

type Resolver struct {
	client *Client
	log    *slog.Logger
}

func New(apiKey string, log *slog.Logger) *Resolver {
	if log == nil {
		log = slog.Default()
	}
	return &Resolver{client: newClient(apiKey), log: log}
}

func (r *Resolver) Resolve(ctx context.Context, keyword, description, alt string) (articles.ResolvedImage, error) {
	query := buildQuery(keyword, description, alt)
	if query == "" {
		return articles.ResolvedImage{}, nil
	}

	photos, err := r.client.SearchN(ctx, query, candidates)
	if err != nil {
		r.log.WarnContext(ctx, "pexels search failed", "query", query, "err", err)
		return articles.ResolvedImage{}, err
	}
	if len(photos) == 0 {
		return articles.ResolvedImage{}, nil
	}

	picked := articles.PickRelevant(toDomainPhotos(photos), keyword, alt)
	if picked == nil {
		r.log.DebugContext(ctx, "pexels: no topical match, skipping image", "query", query, "candidates", len(photos))
		return articles.ResolvedImage{}, nil
	}
	return articles.ResolvedImage{
		URL:             picked.URL,
		Photographer:    picked.Photographer,
		PhotographerURL: picked.PhotographerURL,
		SourceURL:       picked.SourceURL,
	}, nil
}

func toDomainPhotos(photos []Photo) []articles.Photo {
	out := make([]articles.Photo, len(photos))
	for i, p := range photos {
		out[i] = articles.Photo{
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
