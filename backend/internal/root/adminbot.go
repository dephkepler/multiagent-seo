package root

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	appleads "multiagent-seo/internal/application/webleads"
	"multiagent-seo/internal/domain/consultations"
	domainleads "multiagent-seo/internal/domain/webleads"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/infrastructure/sheets"
	"multiagent-seo/internal/infrastructure/telegram"
	"multiagent-seo/pkg/config"
)

func buildAdminBot(ctx context.Context, cfg config.Config, log *slog.Logger, pool *pgxpool.Pool, leadsSvc *appleads.Service) *telegram.AdminBot {
	if cfg.Telegram.BotToken == "" || cfg.Telegram.PaymentCard == "" || len(cfg.Telegram.AllowedUsers) == 0 {
		log.Warn("admin bot disabled: telegram token, payment card, or allowed users not configured")
		return nil
	}

	consultationRepo := postgres.NewConsultationRepository(pool)

	var sheetSink consultations.SheetWriter = noopConsultationSink{}
	if cfg.LeadsSheets.SpreadsheetID != "" {
		sink, err := sheets.NewConsultationSink(ctx, cfg.Sheets.CredentialsFile, cfg.LeadsSheets.SpreadsheetID, cfg.LeadsSheets.ConsultationsSheet, log)
		if err != nil {
			log.Warn("admin bot: consultation sheet sync disabled, spreadsheet unavailable", "err", err)
		} else {
			sheetSink = sink
		}
	}

	var leads telegram.LeadSubmitter = noopLeadSubmitter{}
	if leadsSvc != nil {
		leads = leadsSvc
	} else {
		log.Warn("admin bot: self-booking requests disabled, webleads service unavailable")
	}

	bot, err := telegram.NewAdminBot(
		cfg.Telegram.BotToken,
		cfg.Telegram.PaymentCard,
		cfg.Telegram.AllowedUsers,
		consultationRepo,
		sheetSink,
		leads,
		cfg.Reminder.Before,
		log,
	)
	if err != nil {
		log.Warn("admin bot disabled: telegram client unavailable", "err", err)
		return nil
	}
	return bot
}

// noopConsultationSink is used when CF_LEADS_SHEETS_SPREADSHEET_ID isn't
// set — booking still works, sheet sync is just skipped.
type noopConsultationSink struct{}

func (noopConsultationSink) AppendRow(context.Context, consultations.Consultation, consultations.Client) error {
	return nil
}

type noopLeadSubmitter struct{}

func (noopLeadSubmitter) SubmitLead(context.Context, domainleads.Lead) error {
	return fmt.Errorf("webleads service unavailable")
}
