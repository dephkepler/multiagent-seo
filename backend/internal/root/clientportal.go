package root

import (
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	appclientportal "multiagent-seo/internal/application/clientportal"
	appleads "multiagent-seo/internal/application/webleads"
	"multiagent-seo/internal/domain/consultations"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/pkg/config"
)

// buildClientPortal assembles the client's own section. Returns nil when the
// lead pipeline is unavailable: a request that cannot reach staff is worse than
// a portal that says it is unavailable, because the client walks away believing
// they asked for something.
//
// leads is the concrete type on purpose. buildLeads returns a nil *Service when
// mail or Telegram is unconfigured, and a nil pointer put into an interface
// parameter is not a nil interface — the guard below would pass and the first
// request would panic instead.
func buildClientPortal(
	cfg config.Config,
	log *slog.Logger,
	pool *pgxpool.Pool,
	leads *appleads.Service,
) (*appclientportal.Service, error) {
	if leads == nil {
		log.Warn("client portal disabled: lead pipeline unavailable")
		return nil, nil
	}

	// Fatal, unlike the case above: an unreadable timezone is a typo in the
	// config or a runtime missing its zone database, and neither is a mode
	// anyone chose to run in. It was a warning once, and the result was every
	// booking request answering 503 while the only sign was one line at startup.
	location, err := time.LoadLocation(cfg.Schedule.Timezone)
	if err != nil {
		return nil, fmt.Errorf("schedule timezone %q: %w", cfg.Schedule.Timezone, err)
	}

	repo := postgres.NewConsultationRepository(pool, cfg.Clients.EncryptionKey)
	return appclientportal.NewService(appclientportal.Deps{
		Schedule: consultations.Schedule{
			Location: location,
			Weekdays: consultations.WeekdaysMonToFri,
			Open:     cfg.Schedule.Open,
			Close:    cfg.Schedule.Close,
			Slot:     cfg.Schedule.Slot,
			LeadTime: cfg.Schedule.LeadTime,
			Horizon:  cfg.Schedule.Horizon,
		},
		Consultations: repo,
		Clients:       repo,
		Leads:         leads,
		Log:           log,
	}), nil
}

// isLocalURL reports whether u points at the developer's own machine.
func isLocalURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
