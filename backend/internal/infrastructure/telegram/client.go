package telegram

import (
	"context"
	"fmt"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"multiagent-seo/internal/domain/webleads"
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

func (c *Client) SendMessage(ctx context.Context, text string, buttons []webleads.InlineButton) (int, error) {
	msg := tgbotapi.NewMessage(c.chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	if len(buttons) > 0 {
		// One button per row — several of these labels (see
		// webleads.PracticeAreas) are long enough that packing more than
		// one per row would wrap awkwardly.
		rows := make([][]tgbotapi.InlineKeyboardButton, len(buttons))
		for i, b := range buttons {
			rows[i] = tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(b.Label, b.CallbackData))
		}
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	sent, err := c.bot.Send(msg)
	if err != nil {
		return 0, fmt.Errorf("telegram: send message: %w", err)
	}
	c.log.InfoContext(ctx, "telegram: message sent", "chat_id", c.chatID)
	return sent.MessageID, nil
}
