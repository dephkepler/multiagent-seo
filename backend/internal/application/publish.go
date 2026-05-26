package application

import (
	"context"
	"errors"
	"fmt"

	"contentflow/internal/repo"
	"contentflow/internal/server"
)

func (a *Application) publishArticle(ctx context.Context, id int64) (*server.PublishResult, error) {
	log := a.log.With(
		"request_id", server.RequestIDFromContext(ctx),
		"article_id", id,
	)

	article, err := a.repo.GetArticle(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, server.ErrArticleNotFound
		}
		return nil, fmt.Errorf("fetch article: %w", err)
	}

	switch article.Status {
	case repo.StatusPublished:
		return nil, server.ErrAlreadyPublished
	case repo.StatusGenerating, repo.StatusFailed:
		return nil, server.ErrNoDraftToPublish
	}
	if article.WPPostID == 0 {
		return nil, server.ErrNoDraftToPublish
	}

	pub, ok := a.publisherFor(article.Site)
	if !ok {
		// Stored alias is no longer in YAML — surface it so the operator can restore the entry.
		return nil, fmt.Errorf("%w %q for article %d", server.ErrUnknownSite, article.Site, id)
	}

	postURL, err := pub.Publish(ctx, article.WPPostID)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}

	if err := a.repo.MarkPublished(ctx, id, postURL); err != nil {
		return nil, fmt.Errorf("mark published: %w", err)
	}

	updated, err := a.repo.GetArticle(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("fetch article after publish: %w", err)
	}

	log.Info("article published",
		"site", updated.Site,
		"wp_post_id", updated.WPPostID,
		"wp_post_url", updated.WPPostURL,
	)

	return &server.PublishResult{
		ID:        updated.ID,
		Site:      updated.Site,
		Status:    updated.Status,
		WPPostID:  updated.WPPostID,
		WPEditURL: updated.WPEditURL,
		WPPostURL: updated.WPPostURL,
		UpdatedAt: updated.UpdatedAt,
	}, nil
}
