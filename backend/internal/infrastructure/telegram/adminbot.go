package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"multiagent-seo/internal/domain/consultations"
	domainleads "multiagent-seo/internal/domain/webleads"
)

// LeadSubmitter hands a self-service request off to the same pipeline that
// processes email leads (webleads.Service), so it lands in the same DB
// table, sheet, and leads chat.
type LeadSubmitter interface {
	SubmitLead(ctx context.Context, lead domainleads.Lead) error
}

// The bot has no way to message a client or the advocate directly until
// each has tapped their own /start deep-link once, so for staff-initiated
// bookings the first contact is still sent by hand, outside Telegram.
type AdminBot struct {
	bot            *tgbotapi.BotAPI
	card           string
	allowedUsers   map[int64]bool
	store          consultations.Store
	sheet          consultations.SheetWriter
	leads          LeadSubmitter
	reminderBefore time.Duration
	log            *slog.Logger

	flows map[int64]*flow
}

type flow struct {
	step    string
	consult consultDraft
	book    bookDraft
	request requestDraft
}

type consultDraft struct {
	name string
	date string
	time string
}

type bookDraft struct {
	client consultations.Client
	date   string
	time   string
	price  float64
}

type requestDraft struct {
	name             string
	phone            string
	date             string
	time             string
	telegramUsername string
}

const advocateStartPayload = "advocate"

func NewAdminBot(
	token string,
	cardNumber string,
	allowedUserIDs []int64,
	store consultations.Store,
	sheet consultations.SheetWriter,
	leads LeadSubmitter,
	reminderBefore time.Duration,
	log *slog.Logger,
) (*AdminBot, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram: create admin bot: %w", err)
	}
	if log == nil {
		log = slog.Default()
	}

	// Shown before a first-time user has pressed Telegram's own "Start"
	// button, since that button itself isn't something a bot can add/style.
	_, _ = bot.MakeRequest("setMyDescription", tgbotapi.Params{
		"description": "Бот ТОВ «Абаліс». Натисніть Start, щоб залишити заявку на консультацію з адвокатом.",
	})

	// Default scope covers every client; per-chat scope overrides it for
	// staff, so clients never see admin-only commands in their menu.
	_, _ = bot.Request(tgbotapi.NewSetMyCommandsWithScope(
		tgbotapi.NewBotCommandScopeDefault(),
		tgbotapi.BotCommand{Command: "request", Description: "Забронювати консультацію"},
	))
	staffCommands := []tgbotapi.BotCommand{
		{Command: "invoice", Description: "Згенерувати рахунок на оплату"},
		{Command: "consult", Description: "Згенерувати підтвердження запису на консультацію"},
		{Command: "book", Description: "Забронювати консультацію за Client ID"},
		{Command: "advocate", Description: "Зареєструвати адвоката"},
	}
	for _, id := range allowedUserIDs {
		_, _ = bot.Request(tgbotapi.NewSetMyCommandsWithScope(tgbotapi.NewBotCommandScopeChat(id), staffCommands...))
	}

	allowed := make(map[int64]bool, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		allowed[id] = true
	}

	return &AdminBot{
		bot:            bot,
		card:           cardNumber,
		allowedUsers:   allowed,
		store:          store,
		sheet:          sheet,
		leads:          leads,
		reminderBefore: reminderBefore,
		log:            log,
		flows:          make(map[int64]*flow),
	}, nil
}

// Run polls for updates itself instead of using the library's
// GetUpdatesChan, which logs every failed poll straight to the stdlib
// log package regardless of cause. A 409 Conflict here just means another
// instance (usually prod) already holds this bot token's long-poll slot —
// expected during local dev, not worth a log line every 3 seconds.
func (b *AdminBot) Run(ctx context.Context) {
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		u := tgbotapi.NewUpdate(offset)
		u.Timeout = 60

		updates, err := b.bot.GetUpdates(u)
		if err != nil {
			var tgErr *tgbotapi.Error
			if !errors.As(err, &tgErr) || tgErr.Code != http.StatusConflict {
				b.log.WarnContext(ctx, "telegram getUpdates failed", "error", err)
			}
			time.Sleep(3 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			b.handle(ctx, update)
		}
	}
}

func (b *AdminBot) handle(ctx context.Context, update tgbotapi.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}
	text := strings.TrimSpace(msg.Text)

	// /start comes from whoever tapped the deep-link — client or advocate,
	// never in allowedUsers for a real tap — so this has to run before the
	// admin gate below.
	if strings.HasPrefix(text, "/start") {
		payload := strings.TrimSpace(strings.TrimPrefix(text, "/start"))
		b.handleStart(ctx, msg.Chat.ID, payload, telegramName(msg.From))
		return
	}
	userID, chatID := msg.From.ID, msg.Chat.ID

	// The self-booking flow runs for any client, not just allowedUsers, so
	// it has to be checked before the admin gate below too.
	if text == "/request" || text == requestButtonLabel {
		b.startRequestFlow(ctx, chatID, userID, msg.From)
		return
	}
	if fl := b.flows[userID]; fl != nil && strings.HasPrefix(fl.step, "req_") {
		b.continueRequestFlow(ctx, chatID, userID, fl, text)
		return
	}

	if !msg.Chat.IsPrivate() || !b.allowedUsers[msg.From.ID] {
		return
	}

	switch {
	case strings.HasPrefix(text, "/invoice"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/invoice"))
		if amount, err := parseAmount(arg); err == nil {
			b.sendInvoice(ctx, chatID, amount)
			return
		}
		b.flows[userID] = &flow{step: "invoice_amount"}
		b.send(ctx, chatID, "Введіть суму в грн (наприклад 1500):")
		return

	case text == "/consult":
		b.flows[userID] = &flow{step: "consult_name"}
		b.send(ctx, chatID, "Введіть ПІБ клієнта:")
		return

	case strings.HasPrefix(text, "/book"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/book"))
		if arg == "" {
			b.flows[userID] = &flow{step: "book_client_id"}
			b.send(ctx, chatID, "Введіть Client ID:")
			return
		}
		b.startBooking(ctx, chatID, userID, arg)
		return

	case strings.HasPrefix(text, "/advocate"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/advocate"))
		if arg == "" {
			b.flows[userID] = &flow{step: "advocate_name"}
			b.send(ctx, chatID, "Введіть ПІБ адвоката:")
			return
		}
		b.registerAdvocate(ctx, chatID, arg)
		return
	}

	fl := b.flows[userID]
	if fl == nil {
		return
	}

	switch fl.step {
	case "invoice_amount":
		amount, err := parseAmount(text)
		if err != nil {
			b.send(ctx, chatID, "Не розпізнав суму. Введіть число, наприклад 1500.")
			return
		}
		delete(b.flows, userID)
		b.sendInvoice(ctx, chatID, amount)

	case "consult_name":
		fl.consult.name = text
		fl.step = "consult_date"
		b.send(ctx, chatID, "Введіть дату консультації (наприклад 29.07.2026):")

	case "consult_date":
		fl.consult.date = text
		fl.step = "consult_time"
		b.send(ctx, chatID, "Введіть час консультації (наприклад 15:00):")

	case "consult_time":
		fl.consult.time = text
		fl.step = "consult_amount"
		b.send(ctx, chatID, "Введіть вартість консультації в грн (наприклад 800):")

	case "consult_amount":
		amount, err := parseAmount(text)
		if err != nil {
			b.send(ctx, chatID, "Не розпізнав суму. Введіть число, наприклад 800.")
			return
		}
		msgText := buildConsultText(fl.consult.name, fl.consult.date, fl.consult.time, amount)
		delete(b.flows, userID)
		b.sendHTML(ctx, chatID, msgText)

	case "book_client_id":
		b.startBooking(ctx, chatID, userID, text)

	case "book_date":
		fl.book.date = text
		fl.step = "book_time"
		b.send(ctx, chatID, "Введіть час консультації (наприклад 15:00):")

	case "book_time":
		fl.book.time = text
		fl.step = "book_price"
		b.send(ctx, chatID, "Введіть вартість консультації в грн (наприклад 800):")

	case "book_price":
		amount, err := parseAmount(text)
		if err != nil {
			b.send(ctx, chatID, "Не розпізнав суму. Введіть число, наприклад 800.")
			return
		}
		fl.book.price = amount
		fl.step = "book_case"
		b.send(ctx, chatID, "Введіть справу/коментар (наприклад: справа про мікрокредити, треба позов):")

	case "book_case":
		b.finishBooking(ctx, chatID, userID, fl.book, text)

	case "advocate_name":
		delete(b.flows, userID)
		b.registerAdvocate(ctx, chatID, text)
	}
}

// handleStart links whoever tapped t.me/<bot>?start=<payload> to that
// person, so future notifications can reach them directly. payload is
// either the reserved advocateStartPayload or a Client ID; a bare /start
// (no payload) offers the self-booking button instead.
func (b *AdminBot) handleStart(ctx context.Context, chatID int64, payload, name string) {
	if payload == "" {
		b.sendRequestPrompt(ctx, chatID)
		return
	}

	if payload == advocateStartPayload {
		if err := b.store.SetAdvocateTelegram(ctx, chatID, name); err != nil {
			b.log.WarnContext(ctx, "telegram: advocate start failed", "err", err)
			return
		}
		b.send(ctx, chatID, "Вітаємо! Сповіщення про консультації надходитимуть сюди.")
		return
	}

	// Private-chat ID equals the tapping user's ID, so this catches staff
	// opening a client link themselves instead of forwarding it.
	if b.allowedUsers[chatID] {
		b.send(ctx, chatID, "Це посилання для клієнта — перешліть його, не переходьте самі.")
		return
	}

	client, err := b.store.FindClient(ctx, payload)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: start payload did not match a client", "err", err)
		return
	}
	if err := b.store.SetClientTelegram(ctx, payload, chatID, name); err != nil {
		b.log.WarnContext(ctx, "telegram: set client telegram failed", "err", err)
		return
	}

	c, err := b.store.LatestConsultation(ctx, payload)
	if err != nil {
		b.send(ctx, chatID, "Вас підписано на нагадування щодо консультації. Дякуємо!")
		return
	}
	advocate, err := b.store.GetAdvocate(ctx)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: get advocate failed", "err", err)
	}
	b.sendHTML(ctx, chatID, buildConsultationCard("Вас записано на консультацію", c, client, advocate, true))
}

const requestButtonLabel = "📅 Забронювати консультацію"

func (b *AdminBot) sendRequestPrompt(ctx context.Context, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Вітаємо! Натисніть кнопку нижче, щоб залишити заявку на консультацію.")
	msg.ReplyMarkup = tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(requestButtonLabel),
		),
	)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send request prompt failed", "err", err)
	}
}

func (b *AdminBot) startRequestFlow(ctx context.Context, chatID, userID int64, user *tgbotapi.User) {
	var username string
	if user != nil {
		username = user.UserName
	}
	b.flows[userID] = &flow{step: "req_name", request: requestDraft{telegramUsername: username}}

	msg := tgbotapi.NewMessage(chatID, "Введіть Ваше ПІБ:")
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: start request flow failed", "err", err)
	}
}

func (b *AdminBot) continueRequestFlow(ctx context.Context, chatID, userID int64, fl *flow, text string) {
	switch fl.step {
	case "req_name":
		fl.request.name = text
		fl.step = "req_phone"
		b.send(ctx, chatID, "Введіть Ваш номер телефону:")

	case "req_phone":
		fl.request.phone = text
		fl.step = "req_date"
		b.send(ctx, chatID, "Введіть бажану дату консультації (наприклад 29.07.2026):")

	case "req_date":
		fl.request.date = text
		fl.step = "req_time"
		b.send(ctx, chatID, "Введіть бажаний час (наприклад 15:00):")

	case "req_time":
		fl.request.time = text
		fl.step = "req_question"
		b.send(ctx, chatID, "Опишіть коротко Ваше питання:")

	case "req_question":
		delete(b.flows, userID)
		b.submitRequest(ctx, chatID, fl.request, text)
	}
}

func (b *AdminBot) submitRequest(ctx context.Context, chatID int64, d requestDraft, question string) {
	lead := domainleads.Lead{
		MessageID:        fmt.Sprintf("tg-%d-%d", chatID, time.Now().UnixNano()),
		ReceivedAt:       time.Now(),
		Name:             d.name,
		Phone:            domainleads.NormalizePhone(d.phone),
		Message:          fmt.Sprintf("Бажана дата: %s, час: %s\n\n%s", d.date, d.time, question),
		Page:             "Telegram-бот",
		TelegramUsername: d.telegramUsername,
	}

	if err := b.leads.SubmitLead(ctx, lead); err != nil {
		b.log.ErrorContext(ctx, "telegram: submit self-booking request failed", "err", err)
		b.send(ctx, chatID, "Не вдалося надіслати заявку, спробуйте ще раз пізніше.")
		return
	}
	b.send(ctx, chatID, "Дякуємо! Заявку прийнято, найближчим часом з Вами зв'яжеться наш адвокат.")
}

func (b *AdminBot) registerAdvocate(ctx context.Context, chatID int64, fullName string) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		b.send(ctx, chatID, "ПІБ не може бути порожнім.")
		return
	}
	if _, err := b.store.UpsertAdvocate(ctx, fullName); err != nil {
		b.send(ctx, chatID, "Не вдалося зберегти адвоката, спробуйте ще раз.")
		b.log.ErrorContext(ctx, "telegram: upsert advocate failed", "err", err)
		return
	}
	link := fmt.Sprintf("https://t.me/%s?start=%s", b.bot.Self.UserName, advocateStartPayload)
	b.send(ctx, chatID, "Адвоката збережено. Перешліть йому одноразове посилання:\n\n"+link)
}

func (b *AdminBot) startBooking(ctx context.Context, chatID, userID int64, clientID string) {
	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.flows[userID] = &flow{step: "book_client_id"}
		b.send(ctx, chatID, "Клієнта не знайдено. Введіть Client ID ще раз:")
		return
	}
	b.flows[userID] = &flow{step: "book_date", book: bookDraft{client: client}}
	b.send(ctx, chatID, fmt.Sprintf(
		"Клієнт: %s (%s). Введіть дату консультації (наприклад 29.07.2026):",
		client.Name, client.Phone,
	))
}

func (b *AdminBot) finishBooking(ctx context.Context, chatID, userID int64, draft bookDraft, caseNote string) {
	delete(b.flows, userID)

	scheduledAt, err := time.Parse("02.01.2006 15:04", draft.date+" "+draft.time)
	if err != nil {
		b.send(ctx, chatID, "Не розпізнав дату/час. Спробуйте ще раз: /book "+draft.client.ID)
		return
	}

	c, err := b.store.Save(ctx, consultations.Consultation{
		ClientID:    draft.client.ID,
		ScheduledAt: scheduledAt,
		Price:       draft.price,
		CaseNote:    caseNote,
		CreatedBy:   strconv.FormatInt(userID, 10),
	})
	if err != nil {
		b.send(ctx, chatID, "Не вдалося зберегти консультацію, спробуйте ще раз.")
		b.log.ErrorContext(ctx, "telegram: save consultation failed", "err", err)
		return
	}

	if err := b.sheet.AppendRow(ctx, c, draft.client); err != nil {
		b.log.ErrorContext(ctx, "telegram: append consultation to sheet failed", "err", err)
	}

	// Sent with whatever name the lead form gave us — the advocate gets a
	// more accurate name later via the reminder job, once/if the client
	// taps their own link and we learn their real Telegram identity.
	if advocate, err := b.store.GetAdvocate(ctx); err != nil || advocate.TelegramChatID == 0 {
		b.log.WarnContext(ctx, "telegram: advocate not registered yet, booking notification skipped", "err", err)
	} else {
		b.sendHTML(ctx, advocate.TelegramChatID, buildConsultationCard("📅 Нова консультація заброньована", c, draft.client, consultations.Advocate{}, false))
	}

	link := fmt.Sprintf("https://t.me/%s?start=%s", b.bot.Self.UserName, draft.client.ID)
	b.send(ctx, chatID, "Готово. Перешліть клієнту:\n\n"+link)
}

func (b *AdminBot) SendReminders(ctx context.Context) {
	advocate, err := b.store.GetAdvocate(ctx)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: reminders: advocate lookup failed", "err", err)
	}

	clientTargets, err := b.store.DueClientReminders(ctx, b.reminderBefore)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: due client reminders failed", "err", err)
	}
	for _, t := range clientTargets {
		b.sendHTML(ctx, t.Client.TelegramChatID, buildConsultationCard("⏰ Нагадування про консультацію", t.Consultation, t.Client, advocate, true))
		if err := b.store.MarkClientReminderSent(ctx, t.Consultation.ID); err != nil {
			b.log.ErrorContext(ctx, "telegram: mark client reminder sent failed", "consultation_id", t.Consultation.ID, "err", err)
		}
	}

	if advocate.TelegramChatID == 0 {
		return
	}
	advocateTargets, err := b.store.DueAdvocateReminders(ctx, b.reminderBefore)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: due advocate reminders failed", "err", err)
		return
	}
	for _, t := range advocateTargets {
		b.sendHTML(ctx, advocate.TelegramChatID, buildConsultationCard("⏰ Нагадування про консультацію", t.Consultation, t.Client, consultations.Advocate{}, false))
		if err := b.store.MarkReminderSent(ctx, t.Consultation.ID); err != nil {
			b.log.ErrorContext(ctx, "telegram: mark advocate reminder sent failed", "consultation_id", t.Consultation.ID, "err", err)
		}
	}
}

// showPrice is false for the advocate's own card — the advocate handles
// the legal work, not billing.
func buildConsultationCard(header string, c consultations.Consultation, client consultations.Client, advocate consultations.Advocate, showPrice bool) string {
	card := fmt.Sprintf(
		`%s

Consultation ID: <code>%s</code>
Client ID: <code>%s</code>

Клієнт: %s (%s)
Дата: %s, час: %s
Справа: %s`,
		header,
		c.ID, client.ID,
		html.EscapeString(client.Name), html.EscapeString(client.Phone),
		c.ScheduledAt.Format("02.01.2006"), c.ScheduledAt.Format("15:04"),
		html.EscapeString(c.CaseNote),
	)
	if showPrice {
		card += fmt.Sprintf("\nСума: %s грн", formatAmount(c.Price))
	}
	if advocate.FullName != "" {
		contact := advocate.TelegramUsername
		if contact == "" {
			contact = "—"
		}
		card += fmt.Sprintf("\nАдвокат: %s (%s)", html.EscapeString(advocate.FullName), html.EscapeString(contact))
	}
	return card
}

func telegramName(u *tgbotapi.User) string {
	if u == nil {
		return ""
	}
	if u.UserName != "" {
		return "@" + u.UserName
	}
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

func (b *AdminBot) sendInvoice(ctx context.Context, chatID int64, amount float64) {
	b.send(ctx, chatID, fmt.Sprintf(
		"💳 Рахунок\n\nСума: %s грн\nКартка: %s",
		formatAmount(amount),
		formatCard(b.card),
	))
}

func (b *AdminBot) send(ctx context.Context, chatID int64, text string) {
	if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		b.log.ErrorContext(ctx, "telegram: admin bot send failed", "err", err)
	}
}

func (b *AdminBot) sendHTML(ctx context.Context, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: admin bot send failed", "err", err)
	}
}

const offerURL = "https://abalis.com.ua/publichnyij-dogovor-oferta/"

func buildConsultText(name, date, timeStr string, amount float64) string {
	amountInt := int64(amount)
	words := ukrainianNumberWords(amountInt)
	offerLink := fmt.Sprintf(`<a href="%s">оферта</a>`, offerURL)
	return fmt.Sprintf(
		`%s

Звертаємо Вашу увагу, що запис на консультацію до адвоката ТОВ «Абаліс» на <b>%s %s</b> є погодженням наступних умов:

<b>Вартість консультації складає %d (%s) грн.</b> У разі Вашої неявки на консультацію без попередження про скасування або перенесення не пізніше ніж за 3 години до призначеного часу, Ви зобов'язуєтесь відшкодувати ТОВ «Абаліс» вартість фактично витраченого адвокатом часу в розмірі <b>%d грн</b>, оскільки цей час був заброньований та зарезервований виключно для Вас відповідно до умов Публічного договору (%s).

<b>Оплата здійснюється не пізніше ніж за 2 години до початку консультації або протягом 30 хвилин після її завершення.</b> У разі недотримання цих умов оплати ТОВ «Абаліс» залишає за собою право звернутися із заявою про стягнення заборгованих коштів у судовому порядку.

Листування у месенджері Telegram, включно з підтвердженням дати/часу консультації та цих умов, вважається належною письмовою формою погодження сторін відповідно до ст. 207 Цивільного кодексу України та має юридичну силу нарівні з підписаним документом.

З повагою, ТОВ «Абаліс».`,
		html.EscapeString(name), html.EscapeString(timeStr), html.EscapeString(date), amountInt, words, amountInt, offerLink,
	)
}

func parseAmount(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}
	amount, err := strconv.ParseFloat(s, 64)
	if err != nil || amount <= 0 {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	return amount, nil
}

func formatAmount(amount float64) string {
	if amount == float64(int64(amount)) {
		return strconv.FormatInt(int64(amount), 10)
	}
	return strconv.FormatFloat(amount, 'f', 2, 64)
}

func formatCard(card string) string {
	var b strings.Builder
	for i, r := range card {
		if i > 0 && i%4 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
