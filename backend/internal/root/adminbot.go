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
	"multiagent-seo/internal/infrastructure/telegramuser"
	"multiagent-seo/pkg/config"
)

func buildAdminBot(ctx context.Context, cfg config.Config, log *slog.Logger, pool *pgxpool.Pool, leadsSvc *appleads.Service) *telegram.AdminBot {
	if cfg.Telegram.BotToken == "" || cfg.Telegram.PaymentCard == "" || len(cfg.Telegram.AllowedUsers) == 0 {
		log.Warn("admin bot disabled: telegram token, payment card, or allowed users not configured")
		return nil
	}

	consultationRepo := postgres.NewConsultationRepository(pool, cfg.Clients.EncryptionKey)
	caseRepo := postgres.NewCaseRepository(pool)

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

	// /creategroup needs the personal-account MTProto session (see
	// cmd/tgsession) — the Bot API this bot otherwise runs on has no way to
	// create a group chat at all. Falls back to a noop (command just errors)
	// rather than failing the whole admin bot when it's not configured or
	// the session/connection isn't available.
	var groups telegram.GroupCreator = noopGroupCreator{}
	if cfg.TelegramUser.APIID != 0 && cfg.TelegramUser.APIHash != "" {
		tgUser, err := telegramuser.New(ctx, cfg.TelegramUser.APIID, cfg.TelegramUser.APIHash, cfg.TelegramUser.SessionFile, log)
		if err != nil {
			log.Warn("admin bot: /creategroup disabled, personal Telegram session unavailable", "err", err)
		} else {
			groups = tgUser
		}
	} else {
		log.Warn("admin bot: /creategroup disabled, CF_TELEGRAM_USER_API_ID/HASH not configured")
	}

	bot, err := telegram.NewAdminBot(
		cfg.Telegram.BotToken,
		cfg.Telegram.PaymentCard,
		cfg.Server.AdminURL,
		cfg.Telegram.AllowedUsers,
		consultationRepo,
		sheetSink,
		leads,
		caseRepo,
		groups,
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

type noopGroupCreator struct{}

func (noopGroupCreator) CreateGroup(context.Context, string, []string) (int64, error) {
	return 0, fmt.Errorf("personal Telegram session unavailable")
}
