package webleads

import (
	"html"
	"regexp"
	"strings"
	"time"
)

// The contact form is bilingual (RU/UA), so the same field shows up under
// different labels depending on which language the visitor had selected.
var labelField = map[string]func(*Lead, string){
	"имя":  setName,
	"ім'я": setName,
	"name": setName,

	"номер телефона": setPhone,
	"номер телефону": setPhone,
	"телефон":        setPhone,
	"phone":          setPhone,

	"коментар":     setMessage,
	"комментарий":  setMessage,
	"повідомлення": setMessage,
	"сообщение":    setMessage,
	"message":      setMessage,

	"страница (id)": setPage,
	"сторінка (id)": setPage,
	"страница":      setPage,
	"сторінка":      setPage,
	"page":          setPage,
}

func setName(l *Lead, v string)    { l.Name = v }
func setPhone(l *Lead, v string)   { l.Phone = normalizePhone(v) }
func setMessage(l *Lead, v string) { l.Message = v }
func setPage(l *Lead, v string)    { l.Page = v }

// Block-level tags become line breaks; everything else (e.g. <b>) is just
// removed so it doesn't split a "Label:</b> value" pair onto two lines.
var blockTagRe = regexp.MustCompile(`(?i)</?(p|br|div)[^>]*>`)
var anyTagRe = regexp.MustCompile(`<[^>]*>`)

// If no known label matches, the whole cleaned-up text goes into Message
// rather than being discarded.
func Parse(messageID, fromEmail, subject, body string, receivedAt time.Time) Lead {
	lead := Lead{
		MessageID:  messageID,
		ReceivedAt: receivedAt,
		FromEmail:  fromEmail,
		Subject:    subject,
		RawBody:    body,
	}

	matched := false
	for line := range strings.SplitSeq(stripHTML(body), "\n") {
		label, value, ok := splitLabel(line)
		if !ok {
			continue
		}
		set, known := labelField[strings.ToLower(label)]
		if !known {
			continue
		}
		set(&lead, value)
		matched = true
	}

	if !matched {
		lead.Message = strings.TrimSpace(stripHTML(body))
	}
	return lead
}

func stripHTML(s string) string {
	s = blockTagRe.ReplaceAllString(s, "\n")
	s = anyTagRe.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

func splitLabel(line string) (label, value string, ok bool) {
	line = strings.TrimSpace(line)
	idx := strings.Index(line, ":")
	if line == "" || idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}
