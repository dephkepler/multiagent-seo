package root

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"context"

	appleads "multiagent-seo/internal/application/webleads"
	domainleads "multiagent-seo/internal/domain/webleads"
	"multiagent-seo/internal/infrastructure/mail"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/sheets"
	"multiagent-seo/internal/infrastructure/telegram"
	"multiagent-seo/pkg/config"
)

func buildLeads(ctx context.Context, cfg config.Config, log *slog.Logger, pool *pgxpool.Pool) *appleads.Service {
	if cfg.Mail.IMAPHost == "" || cfg.Telegram.BotToken == "" {
		log.Warn("webleads bot disabled: mail or telegram not configured")
		return nil
	}

	mailClient := mail.New(
		cfg.Mail.IMAPHost,
		cfg.Mail.IMAPPort,
		cfg.Mail.Username,
		cfg.Mail.Password,
		cfg.Mail.Folder,
		log,
	)

	tgClient, err := telegram.New(cfg.Telegram.BotToken, cfg.Telegram.ChatID, log)
	if err != nil {
		log.Warn("webleads bot disabled: telegram client unavailable", "err", err)
		return nil
	}

	var sheetSink domainleads.SheetWriter = noopSheetSink{}
	if cfg.LeadsSheets.SpreadsheetID != "" {
		sheetSink, err = sheets.NewLeadSink(ctx, cfg.Sheets.CredentialsFile, cfg.LeadsSheets.SpreadsheetID, cfg.LeadsSheets.Sheet, log)
		if err != nil {
			log.Warn("webleads: sheet sync disabled, spreadsheet unavailable", "err", err)
			sheetSink = noopSheetSink{}
		}
	}

	leadRepo := postgres.NewLeadRepository(pool)

	return appleads.NewService(mailClient, tgClient, leadRepo, sheetSink, log)
}

type noopSheetSink struct{}

func (noopSheetSink) AppendRow(context.Context, domainleads.Lead) error { return nil }
