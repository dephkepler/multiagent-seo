//go:build integration

package telegram_test

import (
	"context"
	"os"
	"strconv"
	"testing"

	"multiagent-seo/internal/infrastructure/telegram"
)

// TestClient_SendMessage hits the real Telegram Bot API configured via
// CF_TELEGRAM_* env vars and delivers a canned message to the configured
// chat. The full mail -> parse -> format -> send pipeline is covered by
// application/leads' own integration test (ABL 007).
func TestClient_SendMessage(t *testing.T) {
	token := os.Getenv("CF_TELEGRAM_BOT_TOKEN")
	if token == "" {
		t.Skip("CF_TELEGRAM_BOT_TOKEN not set, skipping")
	}
	chatID, err := strconv.ParseInt(os.Getenv("CF_TELEGRAM_CHAT_ID"), 10, 64)
	if err != nil {
		t.Fatalf("invalid CF_TELEGRAM_CHAT_ID: %v", err)
	}

	client, err := telegram.New(token, chatID, nil)
	if err != nil {
		t.Fatalf("telegram.New: %v", err)
	}

	if err := client.SendMessage(context.Background(), "✅ telegram.Client integration test"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
}
