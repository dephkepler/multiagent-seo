package sheets

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/domain/wordpress"
)

// TopicProvider resolves a site's own keyword sheet when it has one
// configured, falling back to a shared default (e.g. the global casino2
// sheet) for every other site.
type TopicProvider struct {
	sites           wordpress.Repository
	credentialsFile string
	defaultSource   articles.TopicSource
	log             *slog.Logger
}

func NewTopicProvider(sites wordpress.Repository, credentialsFile string, defaultSource articles.TopicSource, log *slog.Logger) *TopicProvider {
	if log == nil {
		log = slog.Default()
	}
	return &TopicProvider{sites: sites, credentialsFile: credentialsFile, defaultSource: defaultSource, log: log}
}

func (p *TopicProvider) ForSite(ctx context.Context, siteID uuid.UUID, language string) (articles.TopicSource, error) {
	creds, err := p.sites.Credentials(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("site credentials for site %s: %w", siteID, err)
	}
	lang, ok := creds.Languages[language]
	if !ok || lang.KeywordsSpreadsheetID == "" {
		return p.defaultSource, nil
	}
	return NewClusterColumns(ctx, p.credentialsFile, lang.KeywordsSpreadsheetID, lang.KeywordsSheet, p.log)
}
