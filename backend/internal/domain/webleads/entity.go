package webleads

import (
	"fmt"
	"strings"
	"time"
)

type Lead struct {
	MessageID  string
	ReceivedAt time.Time
	FromEmail  string
	Subject    string
	Name       string
	Phone      string
	Message    string
	Page       string
	RawBody    string

	// Bare username (no "@"), set only for leads submitted through the
	// Telegram bot itself — empty for email-sourced leads.
	TelegramUsername string

	// Empty right after Parse — Store.ResolveClient fills it in (needs a DB lookup).
	ClientID string

	// PracticeArea is set by staff tapping one of PracticeAreaButtons on
	// the lead's own Telegram notification — empty until then (or forever
	// for leads predating this feature).
	PracticeArea string
	// TelegramMessageID is the id of the Telegram message this lead was
	// announced in — filled in by Service.SubmitLead right after
	// Notifier.SendMessage succeeds, before Store.Save. See
	// Store.SetPracticeArea for why this, not MessageID, is the join key
	// a later button tap uses.
	TelegramMessageID int
}

// PracticeAreas mirrors cases.Categories — same taxonomy, deliberately
// duplicated rather than importing the cases domain package for 6 strings
// (see SOLID/DRY/KISS standards: a plain repeat beats a cross-domain
// dependency for something this small). Keep both lists in sync by hand if
// the taxonomy changes.
var PracticeAreas = []string{
	"Мобілізація / ТЦК / ВЛК",
	"Звільнення зі служби / СЗЧ",
	"Борги / аліменти / майнові спори",
	"Оренда / виселення",
	"Виїзд за кордон",
	"Інше",
}

// InlineButton is a generic (label, callback data) pair for a Telegram
// inline keyboard button — kept here, not in infrastructure/telegram, so
// Service can build the practice-area keyboard (see PracticeAreaButtons)
// without depending on the telegram-bot-api package.
type InlineButton struct {
	Label        string
	CallbackData string
}

// PracticeAreaButtons builds one button per PracticeAreas entry.
// callback_data carries only the array index ("leadpa:0".."leadpa:5") —
// never the label text or a lead identifier — so it stays a few bytes
// regardless of label length; AdminBot's "leadpa:" handler resolves which
// lead was tapped from the clicked message's own Telegram id instead (see
// Store.SetPracticeArea).
func PracticeAreaButtons() []InlineButton {
	buttons := make([]InlineButton, len(PracticeAreas))
	for i, area := range PracticeAreas {
		buttons[i] = InlineButton{Label: area, CallbackData: fmt.Sprintf("leadpa:%d", i)}
	}
	return buttons
}

func (l Lead) ShortID() string {
	id, _, _ := strings.Cut(l.MessageID, "@")
	return id
}

type Message struct {
	UID       uint32
	MessageID string
	From      string
	Subject   string
	Date      time.Time
	Body      string
}
