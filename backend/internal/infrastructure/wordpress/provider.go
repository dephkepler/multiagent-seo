package wordpress

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/articles"
	domainwp "multiagent-seo/internal/domain/wordpress"
)

type Provider struct {
	sites domainwp.Repository
	log   *slog.Logger
}

func NewProvider(sites domainwp.Repository, log *slog.Logger) *Provider {
	return &Provider{sites: sites, log: log}
}

func (p *Provider) ForSite(ctx context.Context, siteID uuid.UUID) (articles.Publisher, error) {
	creds, err := p.sites.Credentials(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("wordpress credentials for site %s: %w", siteID, err)
	}
	return New(creds.URL, creds.Username, creds.AppPassword, siteID.String(), p.log), nil
}
