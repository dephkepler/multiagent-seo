package webleads

import (
	"html"
	"strings"
)

// FormatTelegram renders a lead as an HTML-formatted Telegram message.
// Free-form visitor input (Name, Phone, Page, Message) is HTML-escaped —
// unlike the admin bot's own templates, this text comes from random site
// visitors, not staff. Case ID and Client ID are wrapped in <code> so
// tapping them in Telegram copies the value directly, no manual selection.
func FormatTelegram(l Lead) string {
	var b strings.Builder
	b.WriteString("📩 Нова заявка\n")
	writeCodeField(&b, "Case ID", l.ShortID())
	writeCodeField(&b, "Client ID", l.ClientID)
	b.WriteString("\n")

	writeField(&b, "Ім'я", l.Name)
	writeField(&b, "Телефон", l.Phone)
	writeField(&b, "Сторінка", l.Page)

	if strings.TrimSpace(l.Message) != "" {
		b.WriteString("\nКоментар:\n")
		b.WriteString(html.EscapeString(l.Message))
		b.WriteString("\n")
	}

	if !l.ReceivedAt.IsZero() {
		b.WriteString("\n📅 ")
		b.WriteString(l.ReceivedAt.Format("02.01.2006 15:04"))
	}

	return strings.TrimRight(b.String(), "\n")
}

func writeField(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	b.WriteString(label)
	b.WriteString(": ")
	b.WriteString(html.EscapeString(value))
	b.WriteString("\n")
}

func writeCodeField(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	b.WriteString(label)
	b.WriteString(": <code>")
	b.WriteString(html.EscapeString(value))
	b.WriteString("</code>\n")
}
