package wordpress

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/articles"
	domainwp "multiagent-seo/internal/domain/wordpress"
	"multiagent-seo/internal/infrastructure/modx"
)

type Provider struct {
	sites   domainwp.Repository
	log     *slog.Logger
	timeout time.Duration
}

func NewProvider(sites domainwp.Repository, log *slog.Logger, timeout time.Duration) *Provider {
	return &Provider{sites: sites, log: log, timeout: timeout}
}

func (p *Provider) ForSite(ctx context.Context, siteID uuid.UUID, language string) (articles.Publisher, error) {
	creds, err := p.sites.Credentials(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("site credentials for site %s: %w", siteID, err)
	}

	switch creds.Provider {
	case domainwp.ProviderMODX:
		lang, ok := creds.Languages[language]
		if !ok || lang.ContextKey == "" {
			return nil, fmt.Errorf("modx site %s: no context configured for language %q", siteID, language)
		}
		// AppPassword doubles as the agentapi_key here — same generic secret
		// slot WordPress uses for its app password, just interpreted
		// differently per provider.
		pub, err := modx.New(creds.URL, creds.AppPassword, lang.ContextKey, p.log, p.timeout)
		if err != nil {
			return nil, fmt.Errorf("modx publisher for site %s: %w", siteID, err)
		}
		return pub, nil
	default:
		return New(creds.URL, creds.Username, creds.AppPassword, siteID.String(), p.log, p.timeout), nil
	}
}
