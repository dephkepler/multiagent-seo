package webleads_test

import (
	"strings"
	"testing"
	"time"

	"multiagent-seo/internal/domain/webleads"
)

func TestFormatTelegram_AllFields(t *testing.T) {
	lead := webleads.Lead{
		MessageID:  "abc123@abalis.com.ua",
		Name:       "Анна",
		Phone:      "0976004626",
		Page:       "Головна (236)",
		Message:    "Потрібна консультація щодо позову.",
		ReceivedAt: time.Date(2026, 8, 1, 13, 15, 0, 0, time.UTC),
	}

	want := "📩 Нова заявка\n" +
		"Case ID: <code>abc123</code>\n\n" +
		"Ім'я: Анна\n" +
		"Телефон: 0976004626\n" +
		"Сторінка: Головна (236)\n\n" +
		"Коментар:\n" +
		"Потрібна консультація щодо позову.\n\n" +
		"📅 01.08.2026 13:15"

	if got := webleads.FormatTelegram(lead); got != want {
		t.Errorf("FormatTelegram() =\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatTelegram_WithClientID(t *testing.T) {
	lead := webleads.Lead{
		MessageID: "abc123@abalis.com.ua",
		Name:      "Анна",
		ClientID:  "8d5e701c-be40-47a5-b45b-a0502bc300af",
	}

	got := webleads.FormatTelegram(lead)
	if !strings.Contains(got, "Case ID: <code>abc123</code>") {
		t.Errorf("expected Case ID line, got:\n%s", got)
	}
	if !strings.Contains(got, "Client ID: <code>8d5e701c-be40-47a5-b45b-a0502bc300af</code>") {
		t.Errorf("expected Client ID line, got:\n%s", got)
	}
}

func TestFormatTelegram_NoClientIDSkipsLine(t *testing.T) {
	lead := webleads.Lead{MessageID: "abc123@abalis.com.ua", Name: "Анна"}

	got := webleads.FormatTelegram(lead)
	if strings.Contains(got, "Client ID") {
		t.Errorf("expected no Client ID line when ClientID is empty, got:\n%s", got)
	}
}

func TestFormatTelegram_MissingPhoneSkipsLine(t *testing.T) {
	lead := webleads.Lead{
		Name:    "Davyd",
		Message: "тест",
	}

	got := webleads.FormatTelegram(lead)
	if strings.Contains(got, "Телефон") {
		t.Errorf("expected no Телефон line when Phone is empty, got:\n%s", got)
	}
	if !strings.Contains(got, "Ім'я: Davyd") {
		t.Errorf("expected Ім'я line, got:\n%s", got)
	}
}

func TestFormatTelegram_NoMessageSkipsCommentBlock(t *testing.T) {
	lead := webleads.Lead{Name: "Test"}

	got := webleads.FormatTelegram(lead)
	if strings.Contains(got, "Коментар") {
		t.Errorf("expected no Коментар block when Message is empty, got:\n%s", got)
	}
}

func TestFormatTelegram_EscapesHTML(t *testing.T) {
	lead := webleads.Lead{
		Name:    "Іванов <script>",
		Message: "Питання & відповідь",
	}

	got := webleads.FormatTelegram(lead)
	if strings.Contains(got, "<script>") {
		t.Errorf("expected Name to be HTML-escaped, got:\n%s", got)
	}
	if strings.Contains(got, "Питання & відповідь") {
		t.Errorf("expected Message ampersand to be HTML-escaped, got:\n%s", got)
	}
}
