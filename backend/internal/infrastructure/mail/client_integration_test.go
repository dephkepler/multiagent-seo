//go:build integration

package mail_test

import (
	"context"
	"os"
	"strconv"
	"testing"

	"multiagent-seo/internal/infrastructure/mail"
)

// TestClient_FetchUnseen hits the real mailbox configured via CF_MAIL_* env
// vars (see backend/.env) and prints subject/sender for every unseen
// message. Skips if the mailbox isn't configured, so `make test-integration`
// still passes in environments without mail credentials.
func TestClient_FetchUnseen(t *testing.T) {
	host := os.Getenv("CF_MAIL_IMAP_HOST")
	if host == "" {
		t.Skip("CF_MAIL_IMAP_HOST not set, skipping mail integration test")
	}
	port, err := strconv.Atoi(os.Getenv("CF_MAIL_IMAP_PORT"))
	if err != nil {
		t.Fatalf("invalid CF_MAIL_IMAP_PORT: %v", err)
	}

	client := mail.New(
		host,
		port,
		os.Getenv("CF_MAIL_USERNAME"),
		os.Getenv("CF_MAIL_PASSWORD"),
		os.Getenv("CF_MAIL_FOLDER"),
		nil,
	)

	messages, err := client.FetchUnseen(context.Background())
	if err != nil {
		t.Fatalf("FetchUnseen: %v", err)
	}

	t.Logf("unseen messages: %d", len(messages))
	for _, m := range messages {
		t.Logf("- subject=%q from=%q date=%s body_len=%d", m.Subject, m.From, m.Date, len(m.Body))
	}
}
