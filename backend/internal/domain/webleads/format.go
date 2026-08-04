package webleads

import (
	"html"
	"strings"
)

func FormatTelegram(l Lead) string {
	var b strings.Builder
	b.WriteString("📩 Нова заявка\n")
	writeCodeField(&b, "Case ID", l.ShortID())
	writeCodeField(&b, "Client ID", l.ClientID)
	b.WriteString("\n")

	writeField(&b, "Ім'я", l.Name)
	writeField(&b, "Телефон", l.Phone)
	writeField(&b, "Сторінка", l.Page)
	if l.TelegramUsername != "" {
		username := html.EscapeString(l.TelegramUsername)
		b.WriteString("Telegram: <a href=\"https://t.me/")
		b.WriteString(username)
		b.WriteString("\">@")
		b.WriteString(username)
		b.WriteString("</a>\n")
	}

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
