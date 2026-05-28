package wordpress

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/generate"
	domainwp "multiagent-seo/internal/domain/wordpress"
)

var _ generate.PublisherProvider = (*Provider)(nil)

// Provider builds a per-site Publisher by fetching and decrypting that site's
// stored WordPress credentials from the wordpress-sites repository.
type Provider struct {
	sites domainwp.Repository
	log   *slog.Logger
}

func NewProvider(sites domainwp.Repository, log *slog.Logger) *Provider {
	return &Provider{sites: sites, log: log}
}

func (p *Provider) ForSite(ctx context.Context, siteID uuid.UUID) (generate.Publisher, error) {
	creds, err := p.sites.Credentials(ctx, siteID)
	if err != nil {
		return nil, err
	}
	return New(creds.URL, creds.Username, creds.AppPassword, p.log), nil
}
