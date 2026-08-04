package telegram

import (
	"context"
	"fmt"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Client struct {
	bot    *tgbotapi.BotAPI
	chatID int64
	log    *slog.Logger
}

func New(token string, chatID int64, log *slog.Logger) (*Client, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot: %w", err)
	}
	if log == nil {
		log = slog.Default()
	}
	return &Client{bot: bot, chatID: chatID, log: log}, nil
}

func (c *Client) SendMessage(ctx context.Context, text string) error {
	msg := tgbotapi.NewMessage(c.chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := c.bot.Send(msg); err != nil {
		return fmt.Errorf("telegram: send message: %w", err)
	}
	c.log.InfoContext(ctx, "telegram: message sent", "chat_id", c.chatID)
	return nil
}
