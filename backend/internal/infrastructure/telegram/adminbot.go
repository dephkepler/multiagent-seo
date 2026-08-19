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

	"multiagent-seo/internal/domain/cases"
	"multiagent-seo/internal/domain/consultations"
	domainleads "multiagent-seo/internal/domain/webleads"
)

// LeadSubmitter hands a self-service request off to the same pipeline that
// processes email leads (webleads.Service), so it lands in the same DB
// table, sheet, and leads chat. Returns the resolved client id so a caller
// that collected more than Lead carries (e.g. an email) can follow up
// against that same client row.
type LeadSubmitter interface {
	SubmitLead(ctx context.Context, lead domainleads.Lead) (string, error)
}

// GroupCreator makes a Telegram group chat and invites usernames into it.
// Backed by the personal-account MTProto session (see telegramuser) — the
// Bot API this bot otherwise runs on has no method to create a chat at all.
type GroupCreator interface {
	CreateGroup(ctx context.Context, title string, usernames []string) (int64, error)
}

// The bot has no way to message a client or the advocate directly until
// each has tapped their own /start deep-link once, so for staff-initiated
// bookings the first contact is still sent by hand, outside Telegram.
type AdminBot struct {
	bot            *tgbotapi.BotAPI
	card           string
	adminURL       string
	allowedUsers   map[int64]bool
	store          consultations.Store
	sheet          consultations.SheetWriter
	leads          LeadSubmitter
	cases          cases.Store
	groups         GroupCreator
	reminderBefore time.Duration
	log            *slog.Logger

	flows map[int64]*flow
}

type flow struct {
	step        string
	consult     consultDraft
	book        bookDraft
	request     requestDraft
	kase        caseDraft
	pay         payDraft
	newClient   newClientDraft
	creategroup creategroupDraft
}

// newClientDraft holds what resolveClientOrCreate already knows (the typed
// name, and which flow to resume) while its "*_new_phone" step waits on a
// phone number to actually create the row.
type newClientDraft struct {
	name string
	mode clientLookupMode
}

// payDraft holds the Case ID while /pay's wizard waits on the amount —
// same two-step shape as /book and /case, needed because unlike /invoice
// (amount alone), /pay needs a Case ID from staff first.
type payDraft struct {
	caseID string
}

type caseDraft struct {
	client       consultations.Client
	fee          float64
	category     string
	advocateID   string
	advocateName string
}

// creategroupDraft holds the client picked in step 1 while step 2 (advocate
// picker) runs — Telegram's callback_data caps out at 64 bytes, nowhere
// near enough for two UUIDs, so the in-progress pick lives here instead of
// being round-tripped through the button itself.
type creategroupDraft struct {
	clientID   string
	advocateID string
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
	email            string
	category         string
	date             string
	time             string
	telegramUsername string
	// wantsBooking is true only for the "📅 Забронювати консультацію"
	// entry point — it's what makes continueRequestFlow ask for date/time.
	// The plain intake entry point (see intakeStartPayload) collects the
	// same contact fields but isn't tied to a specific slot.
	wantsBooking bool
}

// advocateStartPrefix marks a /start payload as "an advocate tapped their
// personal link", followed by their advocates.id — needed once there's more
// than one advocate, so SetAdvocateTelegram knows which row to update.
const advocateStartPrefix = "advocate_"

// intakeStartPayload is the fixed (not per-client) link staff hands anyone
// they've already talked to elsewhere — SMS, phone, walk-in — to collect
// the same primary-info questions the self-booking flow asks, without
// booking a specific slot. See startIntakeFlow.
const intakeStartPayload = "intake"

func NewAdminBot(
	token string,
	cardNumber string,
	adminURL string,
	allowedUserIDs []int64,
	store consultations.Store,
	sheet consultations.SheetWriter,
	leads LeadSubmitter,
	caseStore cases.Store,
	groups GroupCreator,
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
		{Command: "menu", Description: "Показати кнопки замість команд"},
		{Command: "invoice", Description: "Згенерувати рахунок на оплату"},
		{Command: "consult", Description: "Згенерувати підтвердження запису на консультацію"},
		{Command: "book", Description: "Забронювати консультацію за Client ID"},
		{Command: "advocate", Description: "Додати адвоката"},
		{Command: "advocates", Description: "Список активних адвокатів (і деактивація)"},
		{Command: "case", Description: "Завести справу (клопотання/позов/супровід) за Client ID"},
		{Command: "pay", Description: "Додати оплату по справі: /pay <Case ID> <сума>"},
		{Command: "caseclose", Description: "Позначити справу виконаною: /caseclose <Case ID>"},
		{Command: "creategroup", Description: "Створити групу з клієнтом і адвокатом"},
		{Command: "client", Description: "Знайти клієнта (ім'я/телефон/telegram)"},
		{Command: "intakelink", Description: "Посилання на анкету для нового клієнта"},
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
		adminURL:       strings.TrimSuffix(adminURL, "/"),
		allowedUsers:   allowed,
		store:          store,
		sheet:          sheet,
		leads:          leads,
		cases:          caseStore,
		groups:         groups,
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
	if update.CallbackQuery != nil {
		b.handleCallback(ctx, update.CallbackQuery)
		return
	}

	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}
	text := strings.TrimSpace(msg.Text)

	// /start comes from whoever tapped the deep-link — client or advocate,
	// never in allowedUsers for a real tap — so this has to run before the
	// admin gate below. Staff get their own menu instead of the client
	// "leave a request" prompt — checked first, since a bare /start from
	// staff would otherwise fall into handleStart's client-facing branch.
	if strings.HasPrefix(text, "/start") {
		payload := strings.TrimSpace(strings.TrimPrefix(text, "/start"))
		if payload == "" && b.allowedUsers[msg.From.ID] {
			b.sendStaffMenu(ctx, msg.Chat.ID)
			return
		}
		b.handleStart(ctx, msg.Chat.ID, payload, msg.From)
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

	// A tapped menu button carries its label as plain text — swap it for
	// the equivalent command so every case below (and the flows it starts)
	// runs unchanged, whether staff typed the command or tapped a button.
	if cmd, ok := staffMenuCommands[text]; ok {
		text = cmd
	}

	switch {
	case text == "/menu":
		b.sendStaffMenu(ctx, chatID)
		return

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
			b.send(ctx, chatID, "Введіть Client ID, ім'я або телефон клієнта (якщо не знайдеться — запропоную завести нового):")
			return
		}
		b.startBooking(ctx, chatID, userID, arg)
		return

	// /advocates (list) has to be checked before /advocate (register) —
	// same prefix-collision shape as /caseclose vs /case below.
	case strings.HasPrefix(text, "/advocates"):
		b.listAdvocates(ctx, chatID)
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

	// /caseclose has to be checked before /case — "/caseclose ..." also
	// starts with "/case", so the shorter prefix would swallow it first
	// otherwise and this branch would never fire.
	case strings.HasPrefix(text, "/caseclose"):
		caseID := strings.TrimSpace(strings.TrimPrefix(text, "/caseclose"))
		if caseID == "" {
			b.flows[userID] = &flow{step: "caseclose_id"}
			b.send(ctx, chatID, "Введіть Case ID:")
			return
		}
		b.handleCaseClose(ctx, chatID, caseID)
		return

	case strings.HasPrefix(text, "/case"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/case"))
		if arg == "" {
			b.flows[userID] = &flow{step: "case_client_id"}
			b.send(ctx, chatID, "Введіть Client ID, ім'я або телефон клієнта (якщо не знайдеться — запропоную завести нового):")
			return
		}
		b.startCase(ctx, chatID, userID, arg)
		return

	case strings.HasPrefix(text, "/pay"):
		arg := strings.TrimSpace(strings.TrimPrefix(text, "/pay"))
		if arg == "" {
			b.flows[userID] = &flow{step: "pay_case_id"}
			b.send(ctx, chatID, "Введіть Case ID:")
			return
		}
		b.handlePay(ctx, chatID, arg)
		return

	case strings.HasPrefix(text, "/creategroup"):
		query := strings.TrimSpace(strings.TrimPrefix(text, "/creategroup"))
		if query == "" {
			b.flows[userID] = &flow{step: "creategroup_query"}
			b.send(ctx, chatID, "Введіть ім'я, юзернейм або телефон клієнта:")
			return
		}
		b.searchForGroup(ctx, chatID, query)
		return

	case strings.HasPrefix(text, "/client"):
		query := strings.TrimSpace(strings.TrimPrefix(text, "/client"))
		if query == "" {
			b.flows[userID] = &flow{step: "client_query"}
			b.send(ctx, chatID, "Введіть ім'я, юзернейм або телефон клієнта:")
			return
		}
		b.searchClientInfo(ctx, chatID, query)
		return

	case text == "/intakelink":
		b.sendIntakeLink(ctx, chatID)
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

	case "pay_case_id":
		fl.pay.caseID = text
		fl.step = "pay_amount"
		b.send(ctx, chatID, "Введіть суму в грн (наприклад 1500):")

	case "pay_amount":
		if _, err := parseAmount(text); err != nil {
			b.send(ctx, chatID, "Не розпізнав суму. Введіть число, наприклад 1500.")
			return
		}
		caseID := fl.pay.caseID
		delete(b.flows, userID)
		b.handlePay(ctx, chatID, caseID+" "+text)

	case "caseclose_id":
		delete(b.flows, userID)
		b.handleCaseClose(ctx, chatID, text)

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

	case "case_client_id":
		b.startCase(ctx, chatID, userID, text)

	case "book_new_phone", "case_new_phone":
		client, err := b.store.UpsertClient(ctx, fl.newClient.name, domainleads.NormalizePhone(text))
		if err != nil {
			b.send(ctx, chatID, "Не вдалося створити клієнта, спробуйте ще раз.")
			b.log.ErrorContext(ctx, "telegram: create client failed", "err", err)
			return
		}
		b.continueWithClient(ctx, chatID, userID, client, fl.newClient.mode)

	case "case_fee":
		amount, err := parseAmount(text)
		if err != nil {
			b.send(ctx, chatID, "Не розпізнав суму. Введіть число, наприклад 15000.")
			return
		}
		fl.kase.fee = amount
		fl.step = "case_category"
		b.send(ctx, chatID, "Напрямок справи ("+strings.Join(cases.Categories, " / ")+") — введіть один з варіантів або свій:")

	case "case_category":
		fl.kase.category = text
		b.sendCaseAdvocatePicker(ctx, chatID, userID)

	case "case_description":
		b.finishCase(ctx, chatID, userID, fl.kase, text)

	case "creategroup_query":
		delete(b.flows, userID)
		b.searchForGroup(ctx, chatID, text)

	case "client_query":
		delete(b.flows, userID)
		b.searchClientInfo(ctx, chatID, text)
	}
}

// handleStart links whoever tapped t.me/<bot>?start=<payload> to that
// person, so future notifications can reach them directly. payload is one
// of: empty (bare /start — offers the self-booking button), the reserved
// advocateStartPrefix, intakeStartPayload, or a Client ID.
func (b *AdminBot) handleStart(ctx context.Context, chatID int64, payload string, user *tgbotapi.User) {
	if payload == "" {
		b.sendRequestPrompt(ctx, chatID)
		return
	}
	name := telegramName(user)

	if advocateID, ok := strings.CutPrefix(payload, advocateStartPrefix); ok {
		if err := b.store.SetAdvocateTelegram(ctx, advocateID, chatID, name); err != nil {
			b.log.WarnContext(ctx, "telegram: advocate start failed", "advocate_id", advocateID, "err", err)
			return
		}
		b.send(ctx, chatID, "Вітаємо! Сповіщення про консультації надходитимуть сюди.")
		return
	}

	// Private-chat ID equals the tapping user's ID, so this catches staff
	// opening a client/intake link themselves instead of forwarding it —
	// checked before both branches below (advocate links are exempt: an
	// advocate tapping their own link is the intended flow, not a mistake).
	if b.allowedUsers[chatID] {
		b.send(ctx, chatID, "Це посилання для клієнта — перешліть його, не переходьте самі.")
		return
	}

	// intakeStartPayload is the one link staff sends to anyone they've
	// already talked to outside Telegram — not per-client (nothing about
	// this person is on file yet), so it's a fixed payload, not a prefix
	// with an id like advocateStartPrefix/a Client ID below.
	if payload == intakeStartPayload {
		b.startIntakeFlow(ctx, chatID, user)
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

// Staff reply-keyboard buttons — one per command, so staff tap instead of
// typing/remembering slash-commands. Label text must never collide with
// requestButtonLabel above: that one is checked before the staff gate in
// handle(), so an identical label would misfire into the client flow.
const (
	btnInvoice     = "🧾 Рахунок"
	btnConsult     = "📋 Підтвердження запису"
	btnBook        = "📌 Записати клієнта"
	btnAdvocateAdd = "👤 Додати адвоката"
	btnAdvocates   = "👥 Список адвокатів"
	btnCase        = "📁 Завести справу"
	btnPay         = "💰 Оплата по справі"
	btnCaseClose   = "✅ Закрити справу"
	btnCreateGroup = "👨‍👩‍👧 Створити групу"
	btnClientInfo  = "🔎 Картка клієнта"
	btnIntakeLink  = "📋 Анкета для клієнта"
)

// staffMenuCommands maps a tapped button's label to the equivalent bare
// command — handle() substitutes it before the existing switch, so every
// button reuses that command's logic unchanged (same prompts, same flows).
var staffMenuCommands = map[string]string{
	btnInvoice:     "/invoice",
	btnConsult:     "/consult",
	btnBook:        "/book",
	btnAdvocateAdd: "/advocate",
	btnAdvocates:   "/advocates",
	btnCase:        "/case",
	btnPay:         "/pay",
	btnCaseClose:   "/caseclose",
	btnCreateGroup: "/creategroup",
	btnClientInfo:  "/client",
	btnIntakeLink:  "/intakelink",
}

// Two buttons per row — three squeezed Ukrainian labels onto one row on a
// phone screen (the only place this menu is ever seen), truncating the
// longer ones.
func staffMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(btnClientInfo), tgbotapi.NewKeyboardButton(btnInvoice)),
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(btnConsult), tgbotapi.NewKeyboardButton(btnBook)),
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(btnCreateGroup), tgbotapi.NewKeyboardButton(btnIntakeLink)),
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(btnCase), tgbotapi.NewKeyboardButton(btnPay)),
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(btnCaseClose), tgbotapi.NewKeyboardButton(btnAdvocateAdd)),
		tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(btnAdvocates)),
	)
	kb.ResizeKeyboard = true
	return kb
}

func (b *AdminBot) sendStaffMenu(ctx context.Context, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Меню:")
	msg.ReplyMarkup = staffMenuKeyboard()
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send staff menu failed", "err", err)
	}
}

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
	b.flows[userID] = &flow{step: "req_name", request: requestDraft{telegramUsername: username, wantsBooking: true}}

	msg := tgbotapi.NewMessage(chatID, "Введіть Ваше ПІБ:")
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: start request flow failed", "err", err)
	}
}

// startIntakeFlow is intakeStartPayload's entry point — same requestDraft/
// req_* step machine startRequestFlow uses, minus the date/time steps
// (wantsBooking stays false): this link is about collecting contact info
// for someone staff already talked to, not booking a specific slot.
func (b *AdminBot) startIntakeFlow(ctx context.Context, chatID int64, user *tgbotapi.User) {
	var username string
	userID := chatID // private-chat id equals the tapping user's id; user is the fallback below
	if user != nil {
		username = user.UserName
		userID = user.ID
	}
	b.flows[userID] = &flow{step: "req_name", request: requestDraft{telegramUsername: username}}

	msg := tgbotapi.NewMessage(chatID, "Вітаємо! Кілька коротких запитань — це допоможе адвокату швидше розібратися у Вашій справі.\n\nВведіть Ваше ПІБ:")
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: start intake flow failed", "err", err)
	}
}

// skipLabel is the reply-keyboard button shown next to every optional
// question in the req_* flow — client-facing, so a button beats expecting
// them to type "-" or guess that blank text means "skip".
const skipLabel = "Пропустити"

func (b *AdminBot) continueRequestFlow(ctx context.Context, chatID, userID int64, fl *flow, text string) {
	switch fl.step {
	case "req_name":
		fl.request.name = text
		fl.step = "req_phone"
		b.send(ctx, chatID, "Введіть Ваш номер телефону:")

	case "req_phone":
		fl.request.phone = text
		if fl.request.wantsBooking {
			fl.step = "req_date"
			b.send(ctx, chatID, "Введіть бажану дату консультації (наприклад 29.07.2026):")
			return
		}
		fl.step = "req_email"
		b.sendEmailPrompt(ctx, chatID)

	case "req_date":
		fl.request.date = text
		fl.step = "req_time"
		b.send(ctx, chatID, "Введіть бажаний час (наприклад 15:00):")

	case "req_time":
		fl.request.time = text
		fl.step = "req_email"
		b.sendEmailPrompt(ctx, chatID)

	case "req_email":
		if text != skipLabel {
			fl.request.email = text
		}
		fl.step = "req_category"
		b.sendCategoryPrompt(ctx, chatID)

	case "req_category":
		if text != skipLabel {
			fl.request.category = text
		}
		fl.step = "req_question"
		msg := tgbotapi.NewMessage(chatID, "Опишіть коротко Ваше питання:")
		msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
		if _, err := b.bot.Send(msg); err != nil {
			b.log.ErrorContext(ctx, "telegram: send question prompt failed", "err", err)
		}

	case "req_question":
		delete(b.flows, userID)
		b.submitRequest(ctx, chatID, fl.request, text)
	}
}

func (b *AdminBot) sendEmailPrompt(ctx context.Context, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Введіть Ваш email (необов'язково):")
	msg.ReplyMarkup = tgbotapi.NewReplyKeyboard(tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(skipLabel)))
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send email prompt failed", "err", err)
	}
}

// sendCategoryPrompt offers cases.Categories as buttons — the same
// suggested list staff picks from in /case, so a category left here needs
// no translation to become the one on the case once it's opened.
func (b *AdminBot) sendCategoryPrompt(ctx context.Context, chatID int64) {
	rows := make([][]tgbotapi.KeyboardButton, 0, len(cases.Categories)+1)
	for _, category := range cases.Categories {
		rows = append(rows, tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(category)))
	}
	rows = append(rows, tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(skipLabel)))

	msg := tgbotapi.NewMessage(chatID, "Яка допомога потрібна?")
	kb := tgbotapi.NewReplyKeyboard(rows...)
	kb.ResizeKeyboard = true
	msg.ReplyMarkup = kb
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send category prompt failed", "err", err)
	}
}

// submitRequest hands the collected contact info to the same pipeline
// email leads use (SubmitLead — DB row, sheet, leads-chat notification),
// then separately records email: SubmitLead's Lead has no room for it, and
// blasting it through UpdateClient would overwrite the name/phone
// ResolveClient just set (see SetClientEmail's doc).
func (b *AdminBot) submitRequest(ctx context.Context, chatID int64, d requestDraft, question string) {
	message := question
	if d.category != "" {
		message = fmt.Sprintf("Категорія: %s\n\n%s", d.category, message)
	}
	if d.wantsBooking {
		message = fmt.Sprintf("Бажана дата: %s, час: %s\n\n%s", d.date, d.time, message)
	}

	lead := domainleads.Lead{
		MessageID:        fmt.Sprintf("tg-%d-%d", chatID, time.Now().UnixNano()),
		ReceivedAt:       time.Now(),
		Name:             d.name,
		Phone:            domainleads.NormalizePhone(d.phone),
		Message:          message,
		Page:             requestPage(d.wantsBooking),
		TelegramUsername: d.telegramUsername,
	}

	clientID, err := b.leads.SubmitLead(ctx, lead)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: submit self-service request failed", "err", err)
		b.send(ctx, chatID, "Не вдалося надіслати заявку, спробуйте ще раз пізніше.")
		return
	}

	if d.email != "" && clientID != "" {
		if err := b.store.SetClientEmail(ctx, clientID, d.email); err != nil {
			b.log.WarnContext(ctx, "telegram: set client email from intake failed", "err", err)
		}
	}

	msg := tgbotapi.NewMessage(chatID, "Дякуємо! Заявку прийнято, найближчим часом з Вами зв'яжеться наш адвокат.")
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send request confirmation failed", "err", err)
	}
}

// requestPage distinguishes the two req_* entry points in lead-source
// stats (by_source on the /leads dashboard) — same shape of data, worth
// telling apart since intake leads were never "self-booked".
func requestPage(wantsBooking bool) string {
	if wantsBooking {
		return "Telegram-бот"
	}
	return "Telegram-бот: анкета"
}

func (b *AdminBot) startCase(ctx context.Context, chatID, userID int64, query string) {
	b.resolveClientOrCreate(ctx, chatID, userID, query, lookupForCase)
}

// sendCaseAdvocatePicker replaces the old "type the advocate's full name"
// step — offers the active roster as buttons instead, same pattern as
// /creategroup's advocate step.
func (b *AdminBot) sendCaseAdvocatePicker(ctx context.Context, chatID, userID int64) {
	advocates, err := b.store.ListAdvocates(ctx, true)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: case: list advocates failed", "err", err)
		b.send(ctx, chatID, "Помилка отримання адвокатів, спробуйте ще раз.")
		return
	}
	if len(advocates) == 0 {
		delete(b.flows, userID)
		b.send(ctx, chatID, "Немає активних адвокатів — спочатку /advocate <ПІБ>, потім заново /case.")
		return
	}
	if fl := b.flows[userID]; fl != nil {
		fl.step = "case_advocate_pick"
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(advocates))
	for _, a := range advocates {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(a.FullName, "caseadv:"+a.ID),
		))
	}
	msg := tgbotapi.NewMessage(chatID, "Оберіть адвоката, який веде справу:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send case advocate picker failed", "err", err)
	}
}

func (b *AdminBot) pickCaseAdvocate(ctx context.Context, cb *tgbotapi.CallbackQuery, advocateID string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "case_advocate_pick" {
		b.answerCallback(ctx, cb.ID, "Сесія застаріла, почніть з /case")
		return
	}
	advocate, err := b.findAdvocate(ctx, advocateID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: case: find advocate failed", "advocate_id", advocateID, "err", err)
		b.answerCallback(ctx, cb.ID, "Адвоката не знайдено")
		return
	}
	fl.kase.advocateID = advocateID
	fl.kase.advocateName = advocate.FullName
	fl.step = "case_description"
	b.answerCallback(ctx, cb.ID, "")
	b.editCallbackMessage(ctx, cb, "Адвокат: "+advocate.FullName)
	if cb.Message != nil {
		b.send(ctx, cb.Message.Chat.ID, "Опишіть справу (наприклад: клопотання про відстрочку):")
	}
}

// finishCase auto-links the client's most recent consultation, if any —
// the "which consultation did this case grow out of" question the staff
// member already answered just by using /case for this client right after
// it, so there is no reason to ask again.
func (b *AdminBot) finishCase(ctx context.Context, chatID, userID int64, draft caseDraft, description string) {
	delete(b.flows, userID)

	var consultationID string
	if c, err := b.store.LatestConsultation(ctx, draft.client.ID); err == nil {
		consultationID = c.ID
	}

	saved, err := b.cases.Save(ctx, cases.Case{
		ClientID:       draft.client.ID,
		ConsultationID: consultationID,
		AdvocateID:     draft.advocateID,
		AdvocateName:   draft.advocateName,
		Category:       draft.category,
		Fee:            draft.fee,
		Description:    description,
		CreatedBy:      strconv.FormatInt(userID, 10),
	})
	if err != nil {
		b.send(ctx, chatID, "Не вдалося зберегти справу, спробуйте ще раз.")
		b.log.ErrorContext(ctx, "telegram: save case failed", "err", err)
		return
	}

	b.sendHTML(ctx, chatID, fmt.Sprintf(
		"Готово. Справа <code>%s</code> (%s, %s), сума %s грн.\n\nЩоб додати оплату: /pay <code>%s</code> &lt;сума&gt;\nЩоб позначити виконаною: /caseclose <code>%s</code>",
		saved.ID, html.EscapeString(draft.advocateName), html.EscapeString(draft.category), formatAmount(draft.fee), saved.ID, saved.ID,
	))
}

func (b *AdminBot) handlePay(ctx context.Context, chatID int64, arg string) {
	parts := strings.Fields(arg)
	if len(parts) != 2 {
		b.sendHTML(ctx, chatID, "Формат: /pay &lt;Case ID&gt; &lt;сума&gt;, наприклад /pay <code>3f0b8beb-23c2-4ad4-90fb-48064c9359d4</code> 5000")
		return
	}
	amount, err := parseAmount(parts[1])
	if err != nil {
		b.send(ctx, chatID, "Не розпізнав суму.")
		return
	}
	updated, err := b.cases.AddPayment(ctx, parts[0], amount)
	if err != nil {
		b.send(ctx, chatID, "Не вдалося зберегти оплату — перевірте Case ID.")
		b.log.ErrorContext(ctx, "telegram: add case payment failed", "err", err)
		return
	}
	b.send(ctx, chatID, fmt.Sprintf(
		"Оплата +%s грн. Всього оплачено: %s з %s грн. Залишок: %s грн.",
		formatAmount(amount), formatAmount(updated.PaidAmount), formatAmount(updated.Fee), formatAmount(updated.Owed()),
	))
}

func (b *AdminBot) handleCaseClose(ctx context.Context, chatID int64, caseID string) {
	if caseID == "" {
		b.send(ctx, chatID, "Формат: /caseclose <Case ID>")
		return
	}
	if err := b.cases.UpdateStatus(ctx, caseID, cases.StatusCompleted); err != nil {
		b.send(ctx, chatID, "Не вдалося оновити справу — перевірте Case ID.")
		b.log.ErrorContext(ctx, "telegram: close case failed", "err", err)
		return
	}
	b.send(ctx, chatID, "Справу позначено виконаною ✅")
}

func (b *AdminBot) registerAdvocate(ctx context.Context, chatID int64, fullName string) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		b.send(ctx, chatID, "ПІБ не може бути порожнім.")
		return
	}
	advocate, err := b.store.CreateAdvocate(ctx, fullName)
	if err != nil {
		b.send(ctx, chatID, "Не вдалося зберегти адвоката, спробуйте ще раз.")
		b.log.ErrorContext(ctx, "telegram: create advocate failed", "err", err)
		return
	}
	link := fmt.Sprintf("https://t.me/%s?start=%s%s", b.bot.Self.UserName, advocateStartPrefix, advocate.ID)
	b.send(ctx, chatID, "Адвоката збережено. Перешліть йому одноразове посилання:\n\n"+link)
}

// listAdvocates shows every active advocate with a "деактивувати" button —
// the only way to remove one from pickers short of touching the DB by hand.
func (b *AdminBot) listAdvocates(ctx context.Context, chatID int64) {
	advocates, err := b.store.ListAdvocates(ctx, true)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: list advocates failed", "err", err)
		b.send(ctx, chatID, "Не вдалося отримати список адвокатів.")
		return
	}
	if len(advocates) == 0 {
		b.send(ctx, chatID, "Активних адвокатів немає. Додати: /advocate <ПІБ>")
		return
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(advocates))
	for _, a := range advocates {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ "+a.FullName, "advdeact:"+a.ID),
		))
	}
	msg := tgbotapi.NewMessage(chatID, "Активні адвокати (тап — деактивувати):")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send advocate list failed", "err", err)
	}
}

func (b *AdminBot) deactivateAdvocate(ctx context.Context, cb *tgbotapi.CallbackQuery, advocateID string) {
	if err := b.store.DeactivateAdvocate(ctx, advocateID); err != nil {
		b.log.ErrorContext(ctx, "telegram: deactivate advocate failed", "advocate_id", advocateID, "err", err)
		b.answerCallback(ctx, cb.ID, "Не вдалося деактивувати")
		return
	}
	b.answerCallback(ctx, cb.ID, "Деактивовано")
	b.editCallbackMessage(ctx, cb, "Деактивовано ✅ (старі справи не змінились)")
}

func (b *AdminBot) startBooking(ctx context.Context, chatID, userID int64, query string) {
	b.resolveClientOrCreate(ctx, chatID, userID, query, lookupForBooking)
}

// clientLookupMode picks what happens once resolveClientOrCreate has a
// Client in hand — see continueWithClient.
type clientLookupMode string

const (
	lookupForBooking clientLookupMode = "book"
	lookupForCase    clientLookupMode = "case"
)

// resolveClientOrCreate is /book's and /case's shared first step — staff no
// longer needs a Client ID on hand: an exact id still works (fast path, the
// only thing this used to accept), otherwise the input is treated as a
// name/phone search (see /client's search), and if nothing matches at all,
// offers to create a brand-new client on the spot. That covers "old"
// clients who predate this system and never got an id staff can quote.
func (b *AdminBot) resolveClientOrCreate(ctx context.Context, chatID, userID int64, query string, mode clientLookupMode) {
	if client, err := b.store.FindClient(ctx, query); err == nil {
		b.continueWithClient(ctx, chatID, userID, client, mode)
		return
	}

	matches, err := b.store.SearchClients(ctx, query)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: resolve client: search failed", "err", err)
		b.send(ctx, chatID, "Помилка пошуку, спробуйте ще раз.")
		return
	}
	if len(matches) == 0 {
		b.flows[userID] = &flow{step: string(mode) + "_new_phone", newClient: newClientDraft{name: query, mode: mode}}
		b.send(ctx, chatID, fmt.Sprintf("Клієнта не знайдено. Створити нового на ім'я «%s»? Введіть телефон:", query))
		return
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(matches))
	for _, c := range matches {
		label := c.Name
		if label == "" {
			label = c.TelegramName
		}
		if c.Phone != "" {
			label += " (" + c.Phone + ")"
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "clientpick:"+string(mode)+":"+c.ID),
		))
	}
	msg := tgbotapi.NewMessage(chatID, "Кілька збігів, оберіть клієнта:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send client picker failed", "err", err)
	}
}

// continueWithClient starts whichever flow mode called for once a Client
// is in hand — same two prompts /book and /case already sent, just no
// longer requiring staff to have found the client's id themselves first.
func (b *AdminBot) continueWithClient(ctx context.Context, chatID, userID int64, client consultations.Client, mode clientLookupMode) {
	switch mode {
	case lookupForBooking:
		b.flows[userID] = &flow{step: "book_date", book: bookDraft{client: client}}
		b.send(ctx, chatID, fmt.Sprintf(
			"Клієнт: %s (%s). Введіть дату консультації (наприклад 29.07.2026):",
			client.Name, client.Phone,
		))
	case lookupForCase:
		b.flows[userID] = &flow{step: "case_fee", kase: caseDraft{client: client}}
		b.send(ctx, chatID, fmt.Sprintf(
			"Клієнт: %s (%s). Введіть суму договору в грн (наприклад 15000):",
			client.Name, client.Phone,
		))
	}
}

// pickClientForFlow is the picker's tap handler — resolveClientOrCreate's
// "кілька збігів" case, when more than one search match came back.
func (b *AdminBot) pickClientForFlow(ctx context.Context, cb *tgbotapi.CallbackQuery, mode clientLookupMode, clientID string) {
	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: pick client: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb.ID, "Клієнта не знайдено")
		return
	}
	b.answerCallback(ctx, cb.ID, "")
	b.editCallbackMessage(ctx, cb, "Клієнт: "+client.Name)
	if cb.Message != nil {
		b.continueWithClient(ctx, cb.Message.Chat.ID, cb.From.ID, client, mode)
	}
}

func (b *AdminBot) finishBooking(ctx context.Context, chatID, userID int64, draft bookDraft, caseNote string) {
	delete(b.flows, userID)

	scheduledAt, err := time.Parse("02.01.2006 15:04", draft.date+" "+draft.time)
	if err != nil {
		b.sendHTML(ctx, chatID, "Не розпізнав дату/час. Спробуйте ще раз: /book <code>"+draft.client.ID+"</code>")
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
	msg := tgbotapi.NewMessage(chatID, "Готово. Перешліть клієнту:\n\n"+link)
	msg.ReplyMarkup = statusKeyboard(c.ID)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send booking confirmation failed", "err", err)
	}
}

// statusKeyboard is attached to the booking-confirmation message so staff
// can record the outcome later (after the consultation date has passed)
// without hunting for a command — just tap the button under the message
// they already have.
func statusKeyboard(consultationID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Провів", "cstatus:"+consultationID+":"+consultations.StatusCompleted),
			tgbotapi.NewInlineKeyboardButtonData("❌ Скасував", "cstatus:"+consultationID+":"+consultations.StatusCancelled),
			tgbotapi.NewInlineKeyboardButtonData("🚫 Не прийшов", "cstatus:"+consultationID+":"+consultations.StatusNoShow),
		),
	)
}

func statusLabel(status string) string {
	switch status {
	case consultations.StatusScheduled:
		return "Заплановано"
	case consultations.StatusCompleted:
		return "Провів ✅"
	case consultations.StatusCancelled:
		return "Скасував ❌"
	case consultations.StatusNoShow:
		return "Не прийшов 🚫"
	default:
		return status
	}
}

// handleCallback is the inline-button counterpart of handle — Telegram
// sends a callback query (not a Message) when a button is tapped. Staff-only:
// clients never see these buttons, but the sender is still checked in case a
// button ends up forwarded.
func (b *AdminBot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	if cb.From == nil || !b.allowedUsers[cb.From.ID] {
		b.answerCallback(ctx, cb.ID, "")
		return
	}

	prefix, rest, ok := strings.Cut(cb.Data, ":")
	if !ok {
		b.answerCallback(ctx, cb.ID, "")
		return
	}

	switch prefix {
	case "cstatus":
		consultationID, status, ok := strings.Cut(rest, ":")
		if !ok {
			b.answerCallback(ctx, cb.ID, "")
			return
		}
		b.handleStatusCallback(ctx, cb, consultationID, status)
	case "crgrp":
		// action alone for confirm/cancel (arg == ""), action:arg for
		// pick/adv — see creategroupDraft for why the ids don't both fit
		// in callback_data.
		action, arg, _ := strings.Cut(rest, ":")
		b.handleCreateGroupCallback(ctx, cb, action, arg)
	case "advdeact":
		b.deactivateAdvocate(ctx, cb, rest)
	case "caseadv":
		b.pickCaseAdvocate(ctx, cb, rest)
	case "clinfo":
		b.showClientInfo(ctx, cb, rest)
	case "clientpick":
		mode, clientID, ok := strings.Cut(rest, ":")
		if !ok {
			b.answerCallback(ctx, cb.ID, "")
			return
		}
		b.pickClientForFlow(ctx, cb, clientLookupMode(mode), clientID)
	default:
		b.answerCallback(ctx, cb.ID, "")
	}
}

func (b *AdminBot) handleStatusCallback(ctx context.Context, cb *tgbotapi.CallbackQuery, consultationID, status string) {
	if err := b.store.UpdateStatus(ctx, consultationID, status); err != nil {
		b.log.ErrorContext(ctx, "telegram: update consultation status failed", "err", err)
		b.answerCallback(ctx, cb.ID, "Не вдалося зберегти")
		return
	}
	b.answerCallback(ctx, cb.ID, "Статус: "+statusLabel(status))

	if cb.Message == nil {
		return
	}
	edited := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, cb.Message.Text+"\n\nСтатус: "+statusLabel(status))
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit booking confirmation failed", "err", err)
	}
}

// searchForGroup is /creategroup's entry point — looks a client up by
// whatever the staff member remembers (name, @username, phone) instead of
// making them copy a Client ID, and offers the matches as tappable buttons.
func (b *AdminBot) searchForGroup(ctx context.Context, chatID int64, query string) {
	clients, err := b.store.SearchClients(ctx, query)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: search clients failed", "err", err)
		b.send(ctx, chatID, "Помилка пошуку, спробуйте ще раз.")
		return
	}
	if len(clients) == 0 {
		b.send(ctx, chatID, "Нічого не знайдено. Спробуйте /creategroup ще раз з іншим запитом.")
		return
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(clients))
	for _, c := range clients {
		label := c.Name
		if label == "" {
			label = c.TelegramName
		}
		if c.Phone != "" {
			label += " (" + c.Phone + ")"
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "crgrp:pick:"+c.ID),
		))
	}
	msg := tgbotapi.NewMessage(chatID, "Оберіть клієнта:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send client picker failed", "err", err)
	}
}

// searchClientInfo is /client's entry point — same lookup as
// searchForGroup (name/@username/phone), but tapping a result shows the
// client's own card (IDs to reuse in other commands) instead of feeding
// into the group-creation flow.
// sendIntakeLink hands staff the one static link for anyone they've
// already talked to outside Telegram — unlike /book's link, it's not
// per-client (nothing about this person is on file yet), so there's
// nothing to generate: same URL every time, with ready-to-paste SMS text.
func (b *AdminBot) sendIntakeLink(ctx context.Context, chatID int64) {
	link := fmt.Sprintf("https://t.me/%s?start=%s", b.bot.Self.UserName, intakeStartPayload)
	b.send(ctx, chatID, fmt.Sprintf(
		"Посилання на анкету (не прив'язане до конкретного клієнта — одне на всіх):\n\n%s\n\nГотовий текст для смс:\n\nВітаємо! Натисніть посилання і відповідайте на кілька коротких запитань — це допоможе нашому адвокату швидше розібратися у Вашій справі.\n%s",
		link, link,
	))
}

func (b *AdminBot) searchClientInfo(ctx context.Context, chatID int64, query string) {
	query = strings.TrimSpace(query)

	// Staff often already have a Client ID on hand — copied from a lead
	// notification, a booking confirmation, another card — rather than a
	// name to type. Try it as an exact id first; a query that isn't one
	// simply won't match here and falls through to the search below.
	if client, err := b.store.FindClient(ctx, query); err == nil {
		card, err := b.buildFullClientCard(ctx, client)
		if err != nil {
			b.log.ErrorContext(ctx, "telegram: client info: build card failed", "client_id", client.ID, "err", err)
			b.send(ctx, chatID, "Помилка отримання справ.")
			return
		}
		b.sendHTML(ctx, chatID, card)
		return
	}

	clients, err := b.store.SearchClients(ctx, query)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: client info: search clients failed", "err", err)
		b.send(ctx, chatID, "Помилка пошуку, спробуйте ще раз.")
		return
	}
	if len(clients) == 0 {
		b.send(ctx, chatID, "Нічого не знайдено. Спробуйте /client ще раз з іншим запитом.")
		return
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(clients))
	for _, c := range clients {
		label := c.Name
		if label == "" {
			label = c.TelegramName
		}
		if c.Phone != "" {
			label += " (" + c.Phone + ")"
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "clinfo:"+c.ID),
		))
	}
	msg := tgbotapi.NewMessage(chatID, "Оберіть клієнта:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send client-info picker failed", "err", err)
	}
}

// showClientInfo replaces the picker message with the client's card —
// every ID staff might need to type into another command (Client ID for
// /book and /case, Case ID for /pay and /caseclose, advocate names/IDs),
// plus a link into the CRM for anything not worth cramming into Telegram.
func (b *AdminBot) showClientInfo(ctx context.Context, cb *tgbotapi.CallbackQuery, clientID string) {
	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: client info: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb.ID, "Клієнта не знайдено")
		return
	}
	card, err := b.buildFullClientCard(ctx, client)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: client info: build card failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb.ID, "Помилка отримання справ")
		return
	}

	b.answerCallback(ctx, cb.ID, "")
	b.editCallbackMessageHTML(ctx, cb, card)
}

// buildFullClientCard fetches the rest of what buildClientInfoCard needs
// (latest consultation, every case) and renders the card — shared by the
// picker (showClientInfo) and the exact-Client-ID fast path
// (searchClientInfo), which don't share a caller further up.
func (b *AdminBot) buildFullClientCard(ctx context.Context, client consultations.Client) (string, error) {
	// No consultation yet is the normal case for a client who only just
	// left a request — not an error worth logging.
	latest, err := b.store.LatestConsultation(ctx, client.ID)
	hasConsultation := err == nil

	clientCases, err := b.cases.ListByClient(ctx, client.ID)
	if err != nil {
		return "", fmt.Errorf("list cases for client %q: %w", client.ID, err)
	}

	return buildClientInfoCard(client, latest, hasConsultation, clientCases, b.adminURL), nil
}

// handleCreateGroupCallback drives /creategroup's pick client → pick
// advocate → confirm → create steps. The client id (from step 1) and the
// advocate id (from step 2) live in b.flows[userID], not in callback_data —
// see creategroupDraft.
func (b *AdminBot) handleCreateGroupCallback(ctx context.Context, cb *tgbotapi.CallbackQuery, action, arg string) {
	switch action {
	case "pick":
		b.pickAdvocateForGroup(ctx, cb, arg)
	case "adv":
		b.confirmCreateGroup(ctx, cb, arg)
	case "confirm":
		b.doCreateGroup(ctx, cb)
	case "cancel":
		delete(b.flows, cb.From.ID)
		b.answerCallback(ctx, cb.ID, "Скасовано")
		b.editCallbackMessage(ctx, cb, "Скасовано")
	default:
		b.answerCallback(ctx, cb.ID, "")
	}
}

// pickAdvocateForGroup is step 2: client is chosen, now offer every active
// advocate as a button.
func (b *AdminBot) pickAdvocateForGroup(ctx context.Context, cb *tgbotapi.CallbackQuery, clientID string) {
	if _, err := b.store.FindClient(ctx, clientID); err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb.ID, "Клієнта не знайдено")
		return
	}
	advocates, err := b.store.ListAdvocates(ctx, true)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: creategroup: list advocates failed", "err", err)
		b.answerCallback(ctx, cb.ID, "Помилка отримання адвокатів")
		return
	}
	if len(advocates) == 0 {
		b.answerCallback(ctx, cb.ID, "Немає активних адвокатів")
		b.editCallbackMessage(ctx, cb, "Немає жодного активного адвоката — спочатку /advocate")
		return
	}
	b.flows[cb.From.ID] = &flow{step: "creategroup_advocate", creategroup: creategroupDraft{clientID: clientID}}
	b.answerCallback(ctx, cb.ID, "")

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(advocates))
	for _, a := range advocates {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(a.FullName, "crgrp:adv:"+a.ID),
		))
	}
	if cb.Message == nil {
		return
	}
	edited := tgbotapi.NewEditMessageTextAndMarkup(cb.Message.Chat.ID, cb.Message.MessageID, "Оберіть адвоката:", tgbotapi.NewInlineKeyboardMarkup(rows...))
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit advocate-picker message failed", "err", err)
	}
}

// confirmCreateGroup is step 3: both client and advocate are chosen — show
// the "create for real?" card.
func (b *AdminBot) confirmCreateGroup(ctx context.Context, cb *tgbotapi.CallbackQuery, advocateID string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "creategroup_advocate" {
		b.answerCallback(ctx, cb.ID, "Сесія застаріла, почніть з /creategroup")
		return
	}
	clientID := fl.creategroup.clientID

	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb.ID, "Клієнта не знайдено")
		return
	}
	advocate, err := b.findAdvocate(ctx, advocateID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find advocate failed", "advocate_id", advocateID, "err", err)
		b.answerCallback(ctx, cb.ID, "Адвоката не знайдено")
		return
	}
	fl.creategroup.advocateID = advocateID
	b.answerCallback(ctx, cb.ID, "")

	text := fmt.Sprintf("Створити групу: %s + %s?", client.Name, advocate.FullName)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Створити", "crgrp:confirm"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Скасувати", "crgrp:cancel"),
		),
	)
	if cb.Message == nil {
		return
	}
	edited := tgbotapi.NewEditMessageTextAndMarkup(cb.Message.Chat.ID, cb.Message.MessageID, text, keyboard)
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit group-confirm message failed", "err", err)
	}
}

// findAdvocate looks an advocate up by id — ListAdvocates gives us a slice,
// not a by-id lookup, but the roster is small enough that scanning it is
// simpler than adding a repository method used from exactly one place.
func (b *AdminBot) findAdvocate(ctx context.Context, advocateID string) (consultations.Advocate, error) {
	advocates, err := b.store.ListAdvocates(ctx, false)
	if err != nil {
		return consultations.Advocate{}, err
	}
	for _, a := range advocates {
		if a.ID == advocateID {
			return a, nil
		}
	}
	return consultations.Advocate{}, fmt.Errorf("no advocate with id %q", advocateID)
}

func (b *AdminBot) doCreateGroup(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "creategroup_advocate" || fl.creategroup.advocateID == "" {
		b.answerCallback(ctx, cb.ID, "Сесія застаріла, почніть з /creategroup")
		return
	}
	clientID, advocateID := fl.creategroup.clientID, fl.creategroup.advocateID
	delete(b.flows, cb.From.ID)

	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb.ID, "Клієнта не знайдено")
		return
	}
	advocate, err := b.findAdvocate(ctx, advocateID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find advocate failed", "advocate_id", advocateID, "err", err)
		b.answerCallback(ctx, cb.ID, "Адвоката не знайдено")
		return
	}

	clientUsername, ok := publicUsername(client.TelegramName)
	if !ok {
		b.log.WarnContext(ctx, "telegram: creategroup: client has no public username", "client_id", clientID, "telegram_name", client.TelegramName)
		b.answerCallback(ctx, cb.ID, "У клієнта немає публічного юзернейму")
		b.editCallbackMessage(ctx, cb, "Не можу створити групу: у клієнта немає публічного @username в Telegram (лише ім'я/телефон). Попросіть клієнта встановити юзернейм в налаштуваннях Telegram.")
		return
	}
	if advocate.TelegramUsername == "" {
		b.log.WarnContext(ctx, "telegram: creategroup: advocate has no username", "advocate_id", advocate.ID)
		b.answerCallback(ctx, cb.ID, "У адвоката немає юзернейму")
		b.editCallbackMessage(ctx, cb, "Не можу створити групу: у адвоката немає @username.")
		return
	}

	b.log.InfoContext(ctx, "telegram: creategroup: creating group", "client_id", clientID, "client_username", clientUsername, "advocate_username", advocate.TelegramUsername)
	b.answerCallback(ctx, cb.ID, "Створюю групу...")

	title := "Абаліс: " + client.Name
	if _, err := b.groups.CreateGroup(ctx, title, []string{clientUsername, advocate.TelegramUsername}); err != nil {
		b.log.ErrorContext(ctx, "telegram: creategroup: create group failed", "client_id", clientID, "err", err)
		b.editCallbackMessage(ctx, cb, "Не вдалося створити групу. Деталі — в логах сервера.")
		return
	}
	b.log.InfoContext(ctx, "telegram: creategroup: group created", "client_id", clientID)
	b.editCallbackMessage(ctx, cb, fmt.Sprintf("Групу створено ✅ (%s + %s)", client.Name, advocate.FullName))
}

// publicUsername extracts the bare @username telegramName carries — it's
// either "@handle" (see telegramName()) or a plain display name when the
// person has no public username, in which case ok is false: MTProto can't
// resolve/invite them by name alone.
func publicUsername(telegramName string) (string, bool) {
	return strings.CutPrefix(telegramName, "@")
}

func (b *AdminBot) editCallbackMessage(ctx context.Context, cb *tgbotapi.CallbackQuery, text string) {
	if cb.Message == nil {
		return
	}
	edited := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, text)
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit callback message failed", "err", err)
	}
}

// editCallbackMessageHTML is editCallbackMessage with ModeHTML — needed
// wherever the replacement text uses <code>/<b> (e.g. buildClientInfoCard),
// which plain editCallbackMessage would send as literal angle brackets.
func (b *AdminBot) editCallbackMessageHTML(ctx context.Context, cb *tgbotapi.CallbackQuery, text string) {
	if cb.Message == nil {
		return
	}
	edited := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, text)
	edited.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit callback message failed", "err", err)
	}
}

func (b *AdminBot) answerCallback(ctx context.Context, callbackID, text string) {
	if _, err := b.bot.Request(tgbotapi.NewCallback(callbackID, text)); err != nil {
		b.log.WarnContext(ctx, "telegram: answer callback failed", "err", err)
	}
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

// buildClientInfoCard is /client's result — every ID staff might need to
// paste into another command, plus a link to the full card in the CRM
// (client-facing fields like address/tax ID live there, not here).
func buildClientInfoCard(client consultations.Client, latest consultations.Consultation, hasConsultation bool, clientCases []cases.Case, adminURL string) string {
	telegram := client.TelegramName
	if telegram == "" {
		telegram = "—"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "🔎 <b>Картка клієнта</b>\n\nClient ID: <code>%s</code>\nІм'я: %s\nТелефон: %s\nTelegram: %s\nCRM: %s/clients/%s",
		client.ID, html.EscapeString(client.Name), html.EscapeString(client.Phone), html.EscapeString(telegram),
		adminURL, client.ID,
	)

	b.WriteString("\n\n<b>📅 Консультація</b>\n")
	if hasConsultation {
		fmt.Fprintf(&b, "ID: <code>%s</code>\n%s %s · %s",
			latest.ID, latest.ScheduledAt.Format("02.01.2006"), latest.ScheduledAt.Format("15:04"), statusLabel(latest.Status),
		)
	} else {
		b.WriteString("ще не було")
	}

	fmt.Fprintf(&b, "\n\n<b>📁 Справи (%d)</b>", len(clientCases))
	if len(clientCases) == 0 {
		fmt.Fprintf(&b, "\nще нема — /case <code>%s</code>", client.ID)
	}
	// Each case is its own block (blank line between them) — packed onto
	// one line, this used to run off a phone screen with everything from
	// the category to the amount owed crammed together.
	for _, c := range clientCases {
		advocate := c.AdvocateName
		if advocate == "" {
			advocate = "—"
		}
		advocateID := c.AdvocateID
		if advocateID == "" {
			advocateID = "—"
		}
		fmt.Fprintf(&b, "\n\n<code>%s</code>\n%s\nАдвокат: %s (<code>%s</code>)\n%s/%s грн · залишок %s\nСтатус: %s",
			c.ID, html.EscapeString(c.Category), html.EscapeString(advocate), advocateID,
			formatAmount(c.PaidAmount), formatAmount(c.Fee), formatAmount(c.Owed()), caseStatusLabel(c.Status),
		)
	}

	return b.String()
}

func caseStatusLabel(status string) string {
	switch status {
	case cases.StatusInProgress:
		return "В роботі"
	case cases.StatusCompleted:
		return "Виконано ✅"
	case cases.StatusCancelled:
		return "Скасовано ❌"
	default:
		return status
	}
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
