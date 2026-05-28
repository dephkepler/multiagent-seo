package wordpress

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/articles"
	domainwp "multiagent-seo/internal/domain/wordpress"
)

var _ articles.PublisherProvider = (*Provider)(nil)

// Provider builds a per-site Publisher by fetching and decrypting that site's
// stored WordPress credentials from the wordpress-sites repository.
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
		// Wrap with site_id so the caller's single error log is attributable; the
		// caller (pipeline/Publish) logs it, so don't double-log here.
		return nil, fmt.Errorf("wordpress credentials for site %s: %w", siteID, err)
	}
	return New(creds.URL, creds.Username, creds.AppPassword, siteID.String(), p.log), nil
}
