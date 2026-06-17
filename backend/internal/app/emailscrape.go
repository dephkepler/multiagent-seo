package app

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	appemail "multiagent-seo/internal/application/emailscrape"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/infrastructure/pagefetch"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/sheets"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/jobrunner"
)

func nilableEmailScrapeHandler(svc *appemail.Service) *handlers.EmailScrapeHandler {
	return handlers.NewEmailScrapeHandler(svc)
}

func buildEmailScrape(ctx context.Context, cfg config.Config, log *slog.Logger, pool *pgxpool.Pool) (*appemail.Service, *jobrunner.AsyncRunner) {
	if pool == nil {
		log.Warn("email scrape disabled: database unavailable")
		return nil, nil
	}
	src, err := sheets.NewEmailSource(ctx, cfg.Sheets.CredentialsFile, cfg.Sheets.SpreadsheetID, log)
	if err != nil {
		log.Warn("email scrape disabled: site source unavailable", "err", err)
		return nil, nil
	}
	runner := jobrunner.NewAsyncRunner(cfg.Server.BackgroundJobTimeout, cfg.Server.BackgroundJobConcurrency, log)
	svc := appemail.NewService(
		src,
		pagefetch.New(log),
		postgres.NewScrapedEmailRepository(pool),
		postgres.NewScrapeJobRepository(pool),
		runner,
		log,
	)
	return svc, runner
}
