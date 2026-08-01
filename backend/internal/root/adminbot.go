package root

import (
	"log/slog"

	"multiagent-seo/internal/infrastructure/telegram"
	"multiagent-seo/pkg/config"
)

func buildAdminBot(cfg config.Config, log *slog.Logger) *telegram.AdminBot {
	if cfg.Telegram.BotToken == "" || cfg.Telegram.PaymentCard == "" || len(cfg.Telegram.AllowedUsers) == 0 {
		log.Warn("admin bot disabled: telegram token, payment card, or allowed users not configured")
		return nil
	}

	bot, err := telegram.NewAdminBot(cfg.Telegram.BotToken, cfg.Telegram.PaymentCard, cfg.Telegram.AllowedUsers, log)
	if err != nil {
		log.Warn("admin bot disabled: telegram client unavailable", "err", err)
		return nil
	}
	return bot
}
