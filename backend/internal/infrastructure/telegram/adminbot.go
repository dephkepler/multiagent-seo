package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"multiagent-seo/internal/domain/cases"
	"multiagent-seo/internal/domain/clientsegments"
	"multiagent-seo/internal/domain/consultations"
	domainleads "multiagent-seo/internal/domain/webleads"
)

type LeadSubmitter interface {
	SubmitLead(ctx context.Context, lead domainleads.Lead) (string, error)
}

// backed by the personal-account MTProto session — the Bot API this bot otherwise uses can't create chats
type GroupCreator interface {
	CreateGroup(ctx context.Context, title string, usernames []string) (int64, error)
}

type ClientTagger interface {
	AddTag(ctx context.Context, clientID, tag, createdBy string) error
	RemoveTag(ctx context.Context, clientID, tag string) error
	Tags(ctx context.Context, clientID string) ([]string, error)
	// curated vocabulary — /tags' add picker only offers these, never free text (see sendAddTagPicker)
	ListTagDefs(ctx context.Context) ([]clientsegments.TagDef, error)
}

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
	tags           ClientTagger
	reminderBefore time.Duration
	log            *slog.Logger

	flows map[int64]*flow
	// caches /language choice; unlocked — Run() dispatches updates serially, like flows
	staffLangs map[int64]consultations.StaffLang
}

type flow struct {
	step        string
	consult     consultDraft
	book        bookDraft
	request     requestDraft
	kase        caseDraft
	pay         payDraft
	newClient   newClientDraft
	editClient  editClientDraft
	creategroup creategroupDraft
	tags        tagsDraft
}

type editClientDraft struct {
	clientID string
	field    editableField
}

// clientID lives here, not callback_data — a tag's text can hold ':' and exceed 64 bytes
type tagsDraft struct {
	clientID   string
	clientName string
}

type editableField string

const (
	editFieldLastName   editableField = "last_name"
	editFieldFirstName  editableField = "first_name"
	editFieldPatronymic editableField = "patronymic"
	editFieldPhone      editableField = "phone"
	editFieldEmail      editableField = "email"
)

var editFieldLabel = map[editableField]string{
	editFieldLastName:   "Прізвище",
	editFieldFirstName:  "Ім'я",
	editFieldPatronymic: "По батькові",
	editFieldPhone:      "Телефон",
	editFieldEmail:      "Email",
}

type newClientDraft struct {
	name string
	// pre-filled only by resolveClientOrCreate's phone-lookalike branch
	phone string
	mode  clientLookupMode
}

type payDraft struct {
	caseID string
	amount float64
}

type caseDraft struct {
	client       consultations.Client
	fee          float64
	category     string
	advocateID   string
	advocateName string
}

// clientID lives here instead of callback_data — the 64-byte cap can't fit two UUIDs
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
	client   consultations.Client
	date     string
	time     string
	price    float64
	caseNote string
}

type requestDraft struct {
	name             string
	phone            string
	email            string
	category         string
	date             string
	time             string
	telegramUsername string
	// true only for the booking entry point — gates date/time prompts in continueRequestFlow
	wantsBooking bool
}

const advocateStartPrefix = "advocate_"

// fixed payload, unlike advocateStartPrefix/client IDs — no id exists yet to attach one to
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
	tags ClientTagger,
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

	// shown before the user's first tap on Telegram's Start button; a bot can't restyle it
	_, _ = bot.MakeRequest("setMyDescription", tgbotapi.Params{
		"description": "Бот ТОВ «Абаліс». Натисніть Start, щоб залишити заявку на консультацію з адвокатом.",
	})

	// default scope = client menu; per-chat override below hides admin commands from clients
	_, _ = bot.Request(tgbotapi.NewSetMyCommandsWithScope(
		tgbotapi.NewBotCommandScopeDefault(),
		tgbotapi.BotCommand{Command: "request", Description: "Забронювати консультацію"},
	))
	staffCommands := []tgbotapi.BotCommand{
		{Command: "help", Description: "Довідка — що робить кожна команда"},
		{Command: "menu", Description: "Показати кнопки замість команд"},
		{Command: "invoice", Description: "Згенерувати рахунок на оплату"},
		{Command: "consult", Description: "Згенерувати підтвердження запису на консультацію"},
		{Command: "book", Description: "Записати на консультацію (ім'я/телефон/Client ID)"},
		{Command: "advocate", Description: "Додати адвоката"},
		{Command: "advocates", Description: "Список активних адвокатів (і деактивація)"},
		{Command: "case", Description: "Завести справу (ім'я/телефон/Client ID)"},
		{Command: "pay", Description: "Додати оплату по справі: /pay <Case ID> <сума>"},
		{Command: "caseclose", Description: "Позначити справу виконаною: /caseclose <Case ID>"},
		{Command: "creategroup", Description: "Створити групу з клієнтом і адвокатом"},
		{Command: "client", Description: "Знайти клієнта (ім'я/телефон/telegram)"},
		{Command: "edit", Description: "Редагувати дані клієнта (ім'я, телефон, email)"},
		{Command: "tags", Description: "Мітки клієнта — додати/прибрати вручну"},
		{Command: "intakelink", Description: "Посилання на анкету для нового клієнта"},
		{Command: "language", Description: "Мова бота для Вас особисто (укр/рос)"},
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
		tags:           tags,
		reminderBefore: reminderBefore,
		log:            log,
		flows:          make(map[int64]*flow),
		staffLangs:     make(map[int64]consultations.StaffLang),
	}, nil
}

// polls manually — GetUpdatesChan logs every failure via stdlib log regardless of cause
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
			// 409 means another instance (usually prod) already holds the poll slot — expected locally
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

	// runs before the admin gate — staff get their menu here, else /start falls into the client flow
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

	// must run before the admin gate too — self-booking is open to any client, not just staff
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

	// button label text must map to a command first — ruToUk covers Russian-rendered labels
	if cmd, ok := staffMenuCommands[text]; ok {
		text = cmd
	} else if uk, ok := ruToUk[text]; ok {
		if cmd, ok := staffMenuCommands[uk]; ok {
			text = cmd
		}
	}

	switch {
	case text == "/menu":
		b.sendStaffMenu(ctx, chatID)
		return

	case text == "/help":
		b.sendHTML(ctx, chatID, helpText)
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

	// must come before /advocate — same prefix-collision shape as /caseclose vs /case below
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

	// must come before /case — /caseclose shares that prefix and would be swallowed otherwise
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
		b.handlePay(ctx, chatID, userID, arg)
		return

	case strings.HasPrefix(text, "/creategroup"):
		query := strings.TrimSpace(strings.TrimPrefix(text, "/creategroup"))
		if query == "" {
			b.flows[userID] = &flow{step: "creategroup_query"}
			b.send(ctx, chatID, "Введіть ім'я, юзернейм або телефон клієнта:")
			return
		}
		b.searchForGroup(ctx, chatID, userID, query)
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

	case strings.HasPrefix(text, "/edit"):
		query := strings.TrimSpace(strings.TrimPrefix(text, "/edit"))
		if query == "" {
			b.flows[userID] = &flow{step: "edit_client_query"}
			b.send(ctx, chatID, "Введіть ім'я, телефон або Client ID клієнта:")
			return
		}
		b.searchClientForEdit(ctx, chatID, query)
		return

	case strings.HasPrefix(text, "/tags"):
		query := strings.TrimSpace(strings.TrimPrefix(text, "/tags"))
		if query == "" {
			b.flows[userID] = &flow{step: "tags_client_query"}
			b.send(ctx, chatID, "Введіть ім'я, телефон або Client ID клієнта:")
			return
		}
		b.searchClientForTags(ctx, chatID, userID, query)
		return

	case text == "/intakelink":
		b.sendIntakeLink(ctx, chatID)
		return

	case text == "/language":
		b.sendLanguagePicker(ctx, chatID)
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
		amount, err := parseAmount(text)
		if err != nil {
			b.send(ctx, chatID, "Не розпізнав суму. Введіть число, наприклад 1500.")
			return
		}
		fl.pay.amount = amount
		fl.step = "pay_date"
		b.sendPaymentDatePicker(ctx, chatID)

	case "pay_custom_date":
		paidAt, err := time.Parse("02.01.2006", text)
		if err != nil {
			b.send(ctx, chatID, "Не розпізнав дату. Введіть у форматі 19.08.2026:")
			return
		}
		caseID, amount := fl.pay.caseID, fl.pay.amount
		delete(b.flows, userID)
		b.applyPayment(ctx, chatID, userID, caseID, amount, paidAt)

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
		fl.book.caseNote = text
		fl.step = "book_status"
		b.sendBookingStatusPicker(ctx, chatID)

	case "advocate_name":
		delete(b.flows, userID)
		b.registerAdvocate(ctx, chatID, text)

	case "case_client_id":
		b.startCase(ctx, chatID, userID, text)

	case "new_client_phone":
		b.finishNewClient(ctx, chatID, userID, fl, domainleads.NormalizePhone(text), "", "")

	case "new_client_email":
		b.finishNewClient(ctx, chatID, userID, fl, "", text, "")

	case "new_client_telegram":
		b.finishNewClient(ctx, chatID, userID, fl, "", "", text)

	case "new_client_name_known_phone":
		fl.newClient.name = text
		b.finishNewClient(ctx, chatID, userID, fl, fl.newClient.phone, "", "")

	case "case_fee":
		amount, err := parseAmount(text)
		if err != nil {
			b.send(ctx, chatID, "Не розпізнав суму. Введіть число, наприклад 15000.")
			return
		}
		fl.kase.fee = amount
		fl.step = "case_category"
		// category names stay Ukrainian regardless of language — fixed taxonomy stored on the case
		b.send(ctx, chatID, fmt.Sprintf(b.tr(ctx, chatID, "Напрямок справи (%s) — введіть один з варіантів або свій:"), strings.Join(cases.Categories, " / ")))

	case "case_category":
		fl.kase.category = text
		b.sendCaseAdvocatePicker(ctx, chatID, userID)

	case "case_description":
		b.finishCase(ctx, chatID, userID, fl.kase, text)

	case "creategroup_query":
		delete(b.flows, userID)
		b.searchForGroup(ctx, chatID, userID, text)

	case "client_query":
		delete(b.flows, userID)
		b.searchClientInfo(ctx, chatID, text)

	case "edit_client_query":
		delete(b.flows, userID)
		b.searchClientForEdit(ctx, chatID, text)

	case "edit_field_value":
		clientID, field := fl.editClient.clientID, fl.editClient.field
		delete(b.flows, userID)
		b.applyClientFieldEdit(ctx, chatID, clientID, field, text)

	case "tags_client_query":
		delete(b.flows, userID)
		b.searchClientForTags(ctx, chatID, userID, text)
	}
}

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

	// catches staff opening a client link (chatID = tapping user); advocate links are exempt by design
	if b.allowedUsers[chatID] {
		b.send(ctx, chatID, "Це посилання для клієнта — перешліть його, не переходьте самі.")
		return
	}

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

// label text must never collide with requestButtonLabel, checked before the staff gate in handle()
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
	btnEditClient  = "✏️ Редагувати клієнта"
	btnTags        = "🏷 Мітки клієнта"
	btnIntakeLink  = "📋 Анкета для клієнта"
	btnHelp        = "❓ Довідка"
	btnLanguage    = "🌐 Мова"
)

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
	btnEditClient:  "/edit",
	btnTags:        "/tags",
	btnIntakeLink:  "/intakelink",
	btnHelp:        "/help",
	btnLanguage:    "/language",
}

func (b *AdminBot) staffMenuKeyboard(ctx context.Context, chatID int64) tgbotapi.ReplyKeyboardMarkup {
	btn := func(label string) tgbotapi.KeyboardButton {
		return tgbotapi.NewKeyboardButton(b.tr(ctx, chatID, label))
	}
	kb := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(btn(btnHelp), btn(btnClientInfo)),
		tgbotapi.NewKeyboardButtonRow(btn(btnEditClient), btn(btnTags)),
		tgbotapi.NewKeyboardButtonRow(btn(btnInvoice), btn(btnConsult)),
		tgbotapi.NewKeyboardButtonRow(btn(btnBook), btn(btnCreateGroup)),
		tgbotapi.NewKeyboardButtonRow(btn(btnIntakeLink), btn(btnCase)),
		tgbotapi.NewKeyboardButtonRow(btn(btnPay), btn(btnCaseClose)),
		tgbotapi.NewKeyboardButtonRow(btn(btnAdvocateAdd), btn(btnAdvocates)),
		tgbotapi.NewKeyboardButtonRow(btn(btnLanguage)),
	)
	kb.ResizeKeyboard = true
	return kb
}

const helpText = `📖 <b>Довідка по командах бота</b>

Типовий сценарій: клієнт звернувся → знайти/завести його → записати на консультацію або одразу завести справу → фіксувати оплати → закрити справу.

<b>👤 Клієнти</b>
/client &lt;ім'я/телефон/ID&gt; — знайти клієнта: картка з усіма ID (клієнт/консультація/справи), історія оплат, посилання на CRM.
/edit — виправити ім'я/телефон/email клієнта покроково, кнопками.
/tags — мітки клієнта вручну: додати з готового списку або прибрати (окремо від автоматичних — боржник/ризик неявки тощо).
/book &lt;ім'я/телефон/ID&gt; — записати на консультацію. Клієнта нема в базі — бот сам запропонує завести нового.

<b>📁 Справи</b>
/case &lt;ім'я/телефон/ID&gt; — завести справу (клопотання/позов/супровід). Так само заводить нового клієнта, якщо треба.
/pay — записати оплату по справі: Case ID → сума → дата (сьогодні або вручну). Або одним рядком: /pay &lt;Case ID&gt; &lt;сума&gt; — тоді дата = сьогодні.
/caseclose &lt;Case ID&gt; — позначити справу виконаною.

<b>👨‍⚖️ Адвокати</b>
/advocate &lt;ПІБ&gt; — додати в ростер, дає одноразове посилання для адвоката (перешліть йому).
/advocates — список активних, можна деактивувати (старі справи не зникнуть).

<b>💳 Інше</b>
/invoice &lt;сума&gt; — рахунок на оплату з номером картки, для клієнта.
/consult — текст-підтвердження запису на консультацію (з умовами оферти), для клієнта.
/creategroup — Telegram-група клієнт+адвокат (потрібен зв'язаний особистий акаунт).
/intakelink — універсальне посилання на анкету для нового клієнта (без бронювання конкретної дати) — скиньте у смс/месенджер.

Команда без аргументів сама запитає, що потрібно, крок за кроком — не обов'язково пам'ятати точний синтаксис.

/menu — кнопки замість команд.
/help — ця довідка.`

func (b *AdminBot) sendStaffMenu(ctx context.Context, chatID int64) {
	msg := b.newMsg(ctx, chatID, "Меню:")
	msg.ReplyMarkup = b.staffMenuKeyboard(ctx, chatID)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send staff menu failed", "err", err)
	}
}

func (b *AdminBot) sendLanguagePicker(ctx context.Context, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Оберіть мову бота для себе / Выберите язык бота для себя:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🇺🇦 Українська", "lang:"+string(consultations.StaffLangUK)),
			tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "lang:"+string(consultations.StaffLangRU)),
		),
	)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send language picker failed", "err", err)
	}
}

func (b *AdminBot) pickLanguage(ctx context.Context, cb *tgbotapi.CallbackQuery, lang string) {
	if !consultations.IsStaffLang(lang) {
		b.answerCallback(ctx, cb, "")
		return
	}
	l := consultations.StaffLang(lang)
	if err := b.store.SetStaffLanguage(ctx, cb.From.ID, l); err != nil {
		b.log.ErrorContext(ctx, "telegram: set staff language failed", "err", err)
		b.answerCallback(ctx, cb, "Не вдалося зберегти")
		return
	}
	b.staffLangs[cb.From.ID] = l
	b.answerCallback(ctx, cb, "")
	label := "Мову збережено: Українська ✅"
	if l == consultations.StaffLangRU {
		label = "Язык сохранён: Русский ✅"
	}
	b.editCallbackMessage(ctx, cb, label)

	// Telegram never re-renders a sent reply keyboard — resend the menu or buttons stay stale
	if cb.Message != nil {
		b.sendStaffMenu(ctx, cb.Message.Chat.ID)
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

// mirrors startRequestFlow's req_* steps but skips date/time (wantsBooking stays false)
func (b *AdminBot) startIntakeFlow(ctx context.Context, chatID int64, user *tgbotapi.User) {
	var username string
	userID := chatID // private-chat id == tapping user's id; user param is just a fallback
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

// same category list /case uses — no translation needed since it becomes the case's category
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

// email goes via SetClientEmail — Lead lacks the field, and UpdateClient would clobber name/phone
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

func requestPage(wantsBooking bool) string {
	if wantsBooking {
		return "Telegram-бот"
	}
	return "Telegram-бот: анкета"
}

func (b *AdminBot) startCase(ctx context.Context, chatID, userID int64, query string) {
	b.resolveClientOrCreate(ctx, chatID, userID, query, lookupForCase)
}

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
	msg := b.newMsg(ctx, chatID, "Оберіть адвоката, який веде справу:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send case advocate picker failed", "err", err)
	}
}

func (b *AdminBot) pickCaseAdvocate(ctx context.Context, cb *tgbotapi.CallbackQuery, advocateID string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "case_advocate_pick" {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /case")
		return
	}
	advocate, err := b.findAdvocate(ctx, advocateID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: case: find advocate failed", "advocate_id", advocateID, "err", err)
		b.answerCallback(ctx, cb, "Адвоката не знайдено")
		return
	}
	fl.kase.advocateID = advocateID
	fl.kase.advocateName = advocate.FullName
	fl.step = "case_description"
	b.answerCallback(ctx, cb, "")
	b.editCallbackMessage(ctx, cb, "Адвокат: "+advocate.FullName)
	if cb.Message != nil {
		b.send(ctx, cb.Message.Chat.ID, "Опишіть справу (наприклад: клопотання про відстрочку):")
	}
}

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

	// tr the template before Sprintf fills it — see sendNewClientContactPicker for why order matters
	b.sendHTML(ctx, chatID, fmt.Sprintf(
		b.tr(ctx, chatID, "Готово. Справа <code>%s</code> (%s, %s), сума %s грн.\n\nЩоб додати оплату: /pay <code>%s</code> &lt;сума&gt;\nЩоб позначити виконаною: /caseclose <code>%s</code>"),
		saved.ID, html.EscapeString(draft.advocateName), html.EscapeString(draft.category), formatAmount(draft.fee), saved.ID, saved.ID,
	))
}

func (b *AdminBot) handlePay(ctx context.Context, chatID, userID int64, arg string) {
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
	b.applyPayment(ctx, chatID, userID, parts[0], amount, time.Now())
}

func (b *AdminBot) sendPaymentDatePicker(ctx context.Context, chatID int64) {
	msg := b.newMsg(ctx, chatID, "Коли внесена оплата?")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "📅 Сьогодні"), "paydate:today"),
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "🗓 Інша дата"), "paydate:custom"),
		),
	)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send payment date picker failed", "err", err)
	}
}

func (b *AdminBot) pickPaymentDate(ctx context.Context, cb *tgbotapi.CallbackQuery, choice string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "pay_date" {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /pay")
		return
	}
	b.answerCallback(ctx, cb, "")
	if cb.Message == nil {
		return
	}

	if choice == "custom" {
		fl.step = "pay_custom_date"
		b.editCallbackMessage(ctx, cb, "Введіть дату (наприклад 19.08.2026):")
		return
	}

	caseID, amount := fl.pay.caseID, fl.pay.amount
	delete(b.flows, cb.From.ID)
	b.editCallbackMessage(ctx, cb, "Оплата за сьогодні…")
	b.applyPayment(ctx, cb.Message.Chat.ID, cb.From.ID, caseID, amount, time.Now())
}

func (b *AdminBot) applyPayment(ctx context.Context, chatID, userID int64, caseID string, amount float64, paidAt time.Time) {
	updated, err := b.cases.AddPayment(ctx, caseID, amount, paidAt, strconv.FormatInt(userID, 10))
	if err != nil {
		b.send(ctx, chatID, "Не вдалося зберегти оплату — перевірте Case ID.")
		b.log.ErrorContext(ctx, "telegram: add case payment failed", "err", err)
		return
	}
	b.send(ctx, chatID, fmt.Sprintf(
		b.tr(ctx, chatID, "Оплата +%s грн (%s). Всього оплачено: %s з %s грн. Залишок: %s грн."),
		formatAmount(amount), paidAt.Format("02.01.2006"), formatAmount(updated.PaidAmount), formatAmount(updated.Fee), formatAmount(updated.Owed()),
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
	b.send(ctx, chatID, b.tr(ctx, chatID, "Адвоката збережено. Перешліть йому одноразове посилання:")+"\n\n"+link)
}

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
	msg := b.newMsg(ctx, chatID, "Активні адвокати (тап — деактивувати):")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send advocate list failed", "err", err)
	}
}

func (b *AdminBot) deactivateAdvocate(ctx context.Context, cb *tgbotapi.CallbackQuery, advocateID string) {
	if err := b.store.DeactivateAdvocate(ctx, advocateID); err != nil {
		b.log.ErrorContext(ctx, "telegram: deactivate advocate failed", "advocate_id", advocateID, "err", err)
		b.answerCallback(ctx, cb, "Не вдалося деактивувати")
		return
	}
	b.answerCallback(ctx, cb, "Деактивовано")
	b.editCallbackMessage(ctx, cb, "Деактивовано ✅ (старі справи не змінились)")
}

func (b *AdminBot) startBooking(ctx context.Context, chatID, userID int64, query string) {
	b.resolveClientOrCreate(ctx, chatID, userID, query, lookupForBooking)
}

type clientLookupMode string

const (
	lookupForBooking clientLookupMode = "book"
	lookupForCase    clientLookupMode = "case"
)

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
		// a phone-looking query as name would corrupt fields — SplitName reads it as 3 name parts
		if looksLikePhone(query) {
			b.flows[userID] = &flow{step: "new_client_name_known_phone", newClient: newClientDraft{phone: domainleads.NormalizePhone(query), mode: mode}}
			b.send(ctx, chatID, fmt.Sprintf(b.tr(ctx, chatID, "Клієнта не знайдено. Телефон %s. Введіть ім'я нового клієнта:"), query))
			return
		}
		b.flows[userID] = &flow{step: "new_client_contact", newClient: newClientDraft{name: query, mode: mode}}
		b.sendNewClientContactPicker(ctx, chatID, query)
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
	msg := b.newMsg(ctx, chatID, "Кілька збігів, оберіть клієнта:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send client picker failed", "err", err)
	}
}

func (b *AdminBot) sendNewClientContactPicker(ctx context.Context, chatID int64, name string) {
	// tr runs on the raw template first — a name baked in early won't match ukToRu's key
	text := fmt.Sprintf(b.tr(ctx, chatID, "Клієнта не знайдено. Створити нового на ім'я «%s»? Що є з контактів?"), name)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "📞 Телефон"), "newclient:phone"),
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "✉️ Email"), "newclient:email"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "💬 Telegram"), "newclient:telegram"),
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "⏭ Нічого немає"), "newclient:skip"),
		),
	)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send new client contact picker failed", "err", err)
	}
}

func (b *AdminBot) pickNewClientContact(ctx context.Context, cb *tgbotapi.CallbackQuery, kind string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "new_client_contact" {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть заново")
		return
	}
	b.answerCallback(ctx, cb, "")

	switch kind {
	case "phone":
		fl.step = "new_client_phone"
		b.editCallbackMessage(ctx, cb, "Введіть телефон:")
	case "email":
		fl.step = "new_client_email"
		b.editCallbackMessage(ctx, cb, "Введіть email:")
	case "telegram":
		fl.step = "new_client_telegram"
		b.editCallbackMessage(ctx, cb, "Введіть Telegram (@username або ім'я):")
	case "skip":
		if cb.Message == nil {
			return
		}
		b.editCallbackMessage(ctx, cb, "Клієнта створюю без контактів…")
		b.finishNewClient(ctx, cb.Message.Chat.ID, cb.From.ID, fl, "", "", "")
	}
}

func (b *AdminBot) finishNewClient(ctx context.Context, chatID, userID int64, fl *flow, phone, email, telegramName string) {
	client, err := b.store.CreateClient(ctx, fl.newClient.name, phone, email, telegramName)
	if err != nil {
		b.send(ctx, chatID, "Не вдалося створити клієнта, спробуйте ще раз.")
		b.log.ErrorContext(ctx, "telegram: create client failed", "err", err)
		return
	}
	b.continueWithClient(ctx, chatID, userID, client, fl.newClient.mode)
}

func (b *AdminBot) continueWithClient(ctx context.Context, chatID, userID int64, client consultations.Client, mode clientLookupMode) {
	switch mode {
	case lookupForBooking:
		b.flows[userID] = &flow{step: "book_date", book: bookDraft{client: client}}
		b.send(ctx, chatID, fmt.Sprintf(
			b.tr(ctx, chatID, "Клієнт: %s (%s). Введіть дату консультації (наприклад 29.07.2026):"),
			client.Name, client.Phone,
		))
	case lookupForCase:
		b.flows[userID] = &flow{step: "case_fee", kase: caseDraft{client: client}}
		b.send(ctx, chatID, fmt.Sprintf(
			b.tr(ctx, chatID, "Клієнт: %s (%s). Введіть суму договору в грн (наприклад 15000):"),
			client.Name, client.Phone,
		))
	}
}

func (b *AdminBot) pickClientForFlow(ctx context.Context, cb *tgbotapi.CallbackQuery, mode clientLookupMode, clientID string) {
	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: pick client: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb, "Клієнта не знайдено")
		return
	}
	b.answerCallback(ctx, cb, "")
	b.editCallbackMessage(ctx, cb, "Клієнт: "+client.Name)
	if cb.Message != nil {
		b.continueWithClient(ctx, cb.Message.Chat.ID, cb.From.ID, client, mode)
	}
}

func (b *AdminBot) sendBookingStatusPicker(ctx context.Context, chatID int64) {
	msg := b.newMsg(ctx, chatID, "Це майбутній запис чи консультація вже відбулася (заводите заднім числом)?")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "📅 Майбутня"), "bookstatus:"+consultations.StatusScheduled),
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "✅ Вже відбулася, оплачена"), "bookstatus:"+consultations.StatusCompleted),
		),
	)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send booking status picker failed", "err", err)
	}
}

func (b *AdminBot) pickBookingStatus(ctx context.Context, cb *tgbotapi.CallbackQuery, status string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "book_status" {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /book")
		return
	}
	b.answerCallback(ctx, cb, "")
	if cb.Message == nil {
		return
	}
	b.editCallbackMessage(ctx, cb, statusLabel(status))
	b.finishBooking(ctx, cb.Message.Chat.ID, cb.From.ID, fl.book, status)
}

func (b *AdminBot) finishBooking(ctx context.Context, chatID, userID int64, draft bookDraft, status string) {
	delete(b.flows, userID)

	scheduledAt, err := time.Parse("02.01.2006 15:04", draft.date+" "+draft.time)
	if err != nil {
		b.sendHTML(ctx, chatID, fmt.Sprintf(b.tr(ctx, chatID, "Не розпізнав дату/час. Спробуйте ще раз: /book <code>%s</code>"), draft.client.ID))
		return
	}

	c, err := b.store.Save(ctx, consultations.Consultation{
		ClientID:    draft.client.ID,
		ScheduledAt: scheduledAt,
		Price:       draft.price,
		CaseNote:    draft.caseNote,
		CreatedBy:   strconv.FormatInt(userID, 10),
		Status:      status,
	})
	if err != nil {
		b.send(ctx, chatID, "Не вдалося зберегти консультацію, спробуйте ще раз.")
		b.log.ErrorContext(ctx, "telegram: save consultation failed", "err", err)
		return
	}

	if err := b.sheet.AppendRow(ctx, c, draft.client); err != nil {
		b.log.ErrorContext(ctx, "telegram: append consultation to sheet failed", "err", err)
	}

	link := fmt.Sprintf("https://t.me/%s?start=%s", b.bot.Self.UserName, draft.client.ID)

	var msg tgbotapi.MessageConfig
	if status == consultations.StatusScheduled {
		if advocate, err := b.store.GetAdvocate(ctx); err != nil || advocate.TelegramChatID == 0 {
			b.log.WarnContext(ctx, "telegram: advocate not registered yet, booking notification skipped", "err", err)
		} else {
			b.sendHTML(ctx, advocate.TelegramChatID, buildConsultationCard("📅 Нова консультація заброньована", c, draft.client, consultations.Advocate{}, false))
		}
		msg = tgbotapi.NewMessage(chatID, b.tr(ctx, chatID, "Готово. Перешліть клієнту:")+"\n\n"+link)
		msg.ReplyMarkup = b.statusKeyboard(ctx, chatID, c.ID)
	} else {
		label := b.tr(ctx, chatID, statusLabel(status))
		msg = tgbotapi.NewMessage(chatID, fmt.Sprintf(b.tr(ctx, chatID, "Додано заднім числом (%s). За бажанням перешліть клієнту:\n\n%s"), label, link))
	}
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send booking confirmation failed", "err", err)
	}
}

func (b *AdminBot) statusKeyboard(ctx context.Context, chatID int64, consultationID string) tgbotapi.InlineKeyboardMarkup {
	// Four buttons instead of three: "провів" alone left revenue counted with
	// nobody knowing whether the money arrived. The tokens are short on purpose
	// — callback_data is capped at 64 bytes and "cstatus:" plus a uuid already
	// eats 45 of them.
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "✅ Провів, оплатив"), "cstatus:"+consultationID+":paid"),
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "⏳ Провів, не оплатив"), "cstatus:"+consultationID+":unpaid"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "❌ Скасував"), "cstatus:"+consultationID+":"+consultations.StatusCancelled),
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "🚫 Не прийшов"), "cstatus:"+consultationID+":"+consultations.StatusNoShow),
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

// staff-only: sender is still checked here in case a button ends up forwarded to someone else
func (b *AdminBot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	if cb.From == nil || !b.allowedUsers[cb.From.ID] {
		b.answerCallback(ctx, cb, "")
		return
	}

	prefix, rest, ok := strings.Cut(cb.Data, ":")
	if !ok {
		b.answerCallback(ctx, cb, "")
		return
	}

	switch prefix {
	case "cstatus":
		consultationID, status, ok := strings.Cut(rest, ":")
		if !ok {
			b.answerCallback(ctx, cb, "")
			return
		}
		b.handleStatusCallback(ctx, cb, consultationID, status)
	case "crgrp":
		// action alone for confirm/cancel, action:arg for pick/adv — see creategroupDraft
		action, arg, _ := strings.Cut(rest, ":")
		b.handleCreateGroupCallback(ctx, cb, action, arg)
	case "advdeact":
		b.deactivateAdvocate(ctx, cb, rest)
	case "caseadv":
		b.pickCaseAdvocate(ctx, cb, rest)
	case "clinfo":
		b.showClientInfo(ctx, cb, rest)
	case "editpick":
		b.pickClientForEdit(ctx, cb, rest)
	case "editfield":
		field, clientID, ok := strings.Cut(rest, ":")
		if !ok {
			b.answerCallback(ctx, cb, "")
			return
		}
		b.startEditField(ctx, cb, editableField(field), clientID)
	case "clientpick":
		mode, clientID, ok := strings.Cut(rest, ":")
		if !ok {
			b.answerCallback(ctx, cb, "")
			return
		}
		b.pickClientForFlow(ctx, cb, clientLookupMode(mode), clientID)
	case "bookstatus":
		b.pickBookingStatus(ctx, cb, rest)
	case "newclient":
		b.pickNewClientContact(ctx, cb, rest)
	case "paydate":
		b.pickPaymentDate(ctx, cb, rest)
	case "lang":
		b.pickLanguage(ctx, cb, rest)
	case "tagpick":
		b.pickClientForTags(ctx, cb, rest)
	case "tagdel":
		b.pickRemoveTag(ctx, cb, rest)
	case "tagadd":
		b.pickShowAddTagPicker(ctx, cb)
	case "tagcat":
		b.pickShowCategoryTags(ctx, cb, rest)
	case "tagset":
		b.pickSetTag(ctx, cb, rest)
	case "tagback":
		b.pickTagsBack(ctx, cb)
	case "tagcatback":
		b.pickCategoryBack(ctx, cb)
	default:
		b.answerCallback(ctx, cb, "")
	}
}

// "paid" and "unpaid" are not statuses — both mean completed and carry the
// payment answer with them. Revenue is recognised on the paid one only, so this
// one tap is what stops the P&L booking money that never arrived.
func (b *AdminBot) handleStatusCallback(ctx context.Context, cb *tgbotapi.CallbackQuery, consultationID, status string) {
	var err error
	var label string
	switch status {
	case "paid":
		err = b.store.MarkCompleted(ctx, consultationID, true, time.Now())
		label = b.tr(ctx, cb.From.ID, "Провів, оплатив ✅")
	case "unpaid":
		err = b.store.MarkCompleted(ctx, consultationID, false, time.Time{})
		label = b.tr(ctx, cb.From.ID, "Провів, не оплатив ⏳")
	default:
		err = b.store.UpdateStatus(ctx, consultationID, status)
		label = b.tr(ctx, cb.From.ID, statusLabel(status))
	}
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: update consultation status failed", "err", err)
		b.answerCallback(ctx, cb, "Не вдалося зберегти")
		return
	}
	b.answerCallback(ctx, cb, fmt.Sprintf(b.tr(ctx, cb.From.ID, "Статус: %s"), label))

	if cb.Message == nil {
		return
	}
	suffix := fmt.Sprintf(b.tr(ctx, cb.Message.Chat.ID, "\n\nСтатус: %s"), label)
	edited := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, cb.Message.Text+suffix)
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit booking confirmation failed", "err", err)
	}
}

func (b *AdminBot) searchForGroup(ctx context.Context, chatID, userID int64, query string) {
	// exact-id fast path — SearchClients never matches on id, so this would 404 otherwise
	if client, err := b.store.FindClient(ctx, query); err == nil {
		b.sendGroupAdvocatePicker(ctx, chatID, userID, client.ID)
		return
	}

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
	msg := b.newMsg(ctx, chatID, "Оберіть клієнта:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send client picker failed", "err", err)
	}
}

func (b *AdminBot) sendGroupAdvocatePicker(ctx context.Context, chatID, userID int64, clientID string) {
	advocates, err := b.store.ListAdvocates(ctx, true)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: creategroup: list advocates failed", "err", err)
		b.send(ctx, chatID, "Помилка отримання адвокатів, спробуйте ще раз.")
		return
	}
	if len(advocates) == 0 {
		b.send(ctx, chatID, "Немає активних адвокатів — спочатку /advocate <ПІБ>, потім заново /creategroup.")
		return
	}
	b.flows[userID] = &flow{step: "creategroup_advocate", creategroup: creategroupDraft{clientID: clientID}}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(advocates))
	for _, a := range advocates {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(a.FullName, "crgrp:adv:"+a.ID),
		))
	}
	msg := b.newMsg(ctx, chatID, "Оберіть адвоката:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send group advocate picker failed", "err", err)
	}
}

func (b *AdminBot) sendIntakeLink(ctx context.Context, chatID int64) {
	link := fmt.Sprintf("https://t.me/%s?start=%s", b.bot.Self.UserName, intakeStartPayload)
	b.send(ctx, chatID, fmt.Sprintf(
		"Посилання на анкету (не прив'язане до конкретного клієнта — одне на всіх):\n\n%s\n\nГотовий текст для смс:\n\nВітаємо! Натисніть посилання і відповідайте на кілька коротких запитань — це допоможе нашому адвокату швидше розібратися у Вашій справі.\n%s",
		link, link,
	))
}

func (b *AdminBot) searchClientInfo(ctx context.Context, chatID int64, query string) {
	query = strings.TrimSpace(query)

	// try exact id first — SearchClients doesn't match on id, see searchForGroup
	if client, err := b.store.FindClient(ctx, query); err == nil {
		card, err := b.buildFullClientCard(ctx, chatID, client)
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
	msg := b.newMsg(ctx, chatID, "Оберіть клієнта:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send client-info picker failed", "err", err)
	}
}

func (b *AdminBot) showClientInfo(ctx context.Context, cb *tgbotapi.CallbackQuery, clientID string) {
	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: client info: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb, "Клієнта не знайдено")
		return
	}
	card, err := b.buildFullClientCard(ctx, cb.From.ID, client)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: client info: build card failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb, "Помилка отримання справ")
		return
	}

	b.answerCallback(ctx, cb, "")
	b.editCallbackMessageHTML(ctx, cb, card)
}

func (b *AdminBot) buildFullClientCard(ctx context.Context, chatID int64, client consultations.Client) (string, error) {
	// no consultation yet is normal for a brand-new client — not logged as an error
	latest, err := b.store.LatestConsultation(ctx, client.ID)
	hasConsultation := err == nil

	clientCases, err := b.cases.ListByClient(ctx, client.ID)
	if err != nil {
		return "", fmt.Errorf("list cases for client %q: %w", client.ID, err)
	}

	tr := func(s string) string { return b.tr(ctx, chatID, s) }
	return buildClientInfoCard(client, latest, hasConsultation, clientCases, b.adminURL, tr), nil
}

func (b *AdminBot) searchClientForEdit(ctx context.Context, chatID int64, query string) {
	query = strings.TrimSpace(query)

	if client, err := b.store.FindClient(ctx, query); err == nil {
		b.sendEditFieldPicker(ctx, chatID, client)
		return
	}

	clients, err := b.store.SearchClients(ctx, query)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: edit client: search failed", "err", err)
		b.send(ctx, chatID, "Помилка пошуку, спробуйте ще раз.")
		return
	}
	if len(clients) == 0 {
		b.send(ctx, chatID, "Нічого не знайдено. Спробуйте /edit ще раз з іншим запитом.")
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
			tgbotapi.NewInlineKeyboardButtonData(label, "editpick:"+c.ID),
		))
	}
	msg := b.newMsg(ctx, chatID, "Оберіть клієнта:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send edit client picker failed", "err", err)
	}
}

func (b *AdminBot) pickClientForEdit(ctx context.Context, cb *tgbotapi.CallbackQuery, clientID string) {
	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: edit client: find failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb, "Клієнта не знайдено")
		return
	}
	b.answerCallback(ctx, cb, "")
	if cb.Message == nil {
		return
	}
	b.editCallbackMessage(ctx, cb, "Клієнт: "+client.Name)
	b.sendEditFieldPicker(ctx, cb.Message.Chat.ID, client)
}

func (b *AdminBot) sendEditFieldPicker(ctx context.Context, chatID int64, client consultations.Client) {
	fields := []editableField{editFieldLastName, editFieldFirstName, editFieldPatronymic, editFieldPhone, editFieldEmail}
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(fields))
	for _, f := range fields {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, editFieldLabel[f]), "editfield:"+string(f)+":"+client.ID),
		))
	}
	msg := b.newMsg(ctx, chatID, "Що змінити?")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send edit field picker failed", "err", err)
	}
}

func (b *AdminBot) startEditField(ctx context.Context, cb *tgbotapi.CallbackQuery, field editableField, clientID string) {
	label, ok := editFieldLabel[field]
	if !ok {
		b.answerCallback(ctx, cb, "")
		return
	}
	b.answerCallback(ctx, cb, "")
	b.flows[cb.From.ID] = &flow{step: "edit_field_value", editClient: editClientDraft{clientID: clientID, field: field}}
	if cb.Message != nil {
		chatID := cb.Message.Chat.ID
		prompt := fmt.Sprintf(b.tr(ctx, chatID, "Введіть нове значення для «%s»:"), b.tr(ctx, chatID, label))
		b.editCallbackMessage(ctx, cb, prompt)
	}
}

// uses a single-column setter, never UpdateClient — that could blank the rest of the client
func (b *AdminBot) applyClientFieldEdit(ctx context.Context, chatID int64, clientID string, field editableField, value string) {
	value = strings.TrimSpace(value)

	var err error
	switch field {
	case editFieldLastName:
		err = b.store.SetClientNamePart(ctx, clientID, consultations.NamePartLast, value)
	case editFieldFirstName:
		err = b.store.SetClientNamePart(ctx, clientID, consultations.NamePartFirst, value)
	case editFieldPatronymic:
		err = b.store.SetClientNamePart(ctx, clientID, consultations.NamePartPatronymic, value)
	case editFieldPhone:
		err = b.store.SetClientPhone(ctx, clientID, domainleads.NormalizePhone(value))
	case editFieldEmail:
		err = b.store.SetClientEmail(ctx, clientID, value)
	default:
		return
	}
	if err != nil {
		b.send(ctx, chatID, "Не вдалося оновити дані клієнта, спробуйте ще раз.")
		b.log.ErrorContext(ctx, "telegram: update client field failed", "field", field, "err", err)
		return
	}
	b.send(ctx, chatID, "Оновлено ✅")
}

func (b *AdminBot) searchClientForTags(ctx context.Context, chatID, userID int64, query string) {
	query = strings.TrimSpace(query)

	if client, err := b.store.FindClient(ctx, query); err == nil {
		b.sendTagsMenu(ctx, chatID, userID, client.ID, client.Name)
		return
	}

	clients, err := b.store.SearchClients(ctx, query)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: tags: search failed", "err", err)
		b.send(ctx, chatID, "Помилка пошуку, спробуйте ще раз.")
		return
	}
	if len(clients) == 0 {
		b.send(ctx, chatID, "Нічого не знайдено. Спробуйте /tags ще раз з іншим запитом.")
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
			tgbotapi.NewInlineKeyboardButtonData(label, "tagpick:"+c.ID),
		))
	}
	msg := b.newMsg(ctx, chatID, "Оберіть клієнта:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send tags client picker failed", "err", err)
	}
}

func (b *AdminBot) pickClientForTags(ctx context.Context, cb *tgbotapi.CallbackQuery, clientID string) {
	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: tags: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb, "Клієнта не знайдено")
		return
	}
	b.answerCallback(ctx, cb, "")
	if cb.Message == nil {
		return
	}
	b.sendTagsMenu(ctx, cb.Message.Chat.ID, cb.From.ID, client.ID, client.Name)
}

// shared by sendTagsMenu and refreshTagsMenu so the two screens can't drift apart
func (b *AdminBot) renderTagsMenu(ctx context.Context, chatID int64, clientID, clientName string) (string, tgbotapi.InlineKeyboardMarkup, error) {
	tags, err := b.tags.Tags(ctx, clientID)
	if err != nil {
		return "", tgbotapi.InlineKeyboardMarkup{}, err
	}
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(tags)+1)
	for _, t := range tags {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ "+t, "tagdel:"+t),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "➕ Додати тег"), "tagadd"),
	))
	text := fmt.Sprintf(b.tr(ctx, chatID, "🏷 Мітки клієнта: %s"), clientName)
	if len(tags) == 0 {
		text += "\n" + b.tr(ctx, chatID, "Міток ще немає.")
	}
	return text, tgbotapi.NewInlineKeyboardMarkup(rows...), nil
}

func (b *AdminBot) sendTagsMenu(ctx context.Context, chatID, userID int64, clientID, clientName string) {
	if b.tags == nil {
		b.send(ctx, chatID, "Мітки клієнта недоступні.")
		return
	}
	text, markup, err := b.renderTagsMenu(ctx, chatID, clientID, clientName)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: tags: list failed", "client_id", clientID, "err", err)
		b.send(ctx, chatID, "Помилка отримання міток, спробуйте ще раз.")
		return
	}

	b.flows[userID] = &flow{step: "tags_menu", tags: tagsDraft{clientID: clientID, clientName: clientName}}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = markup
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: send tags menu failed", "err", err)
	}
}

// clientID/clientName come from the flow, not re-fetched — /tags stays on tags_menu for this client
func (b *AdminBot) refreshTagsMenu(ctx context.Context, cb *tgbotapi.CallbackQuery, clientID, clientName string) {
	chatID := cb.Message.Chat.ID
	text, markup, err := b.renderTagsMenu(ctx, chatID, clientID, clientName)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: tags: refresh list failed", "client_id", clientID, "err", err)
		return
	}
	edited := tgbotapi.NewEditMessageTextAndMarkup(chatID, cb.Message.MessageID, text, markup)
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit tags menu failed", "err", err)
	}
}

func (b *AdminBot) pickRemoveTag(ctx context.Context, cb *tgbotapi.CallbackQuery, tag string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "tags_menu" || b.tags == nil {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /tags")
		return
	}
	if err := b.tags.RemoveTag(ctx, fl.tags.clientID, tag); err != nil {
		b.log.ErrorContext(ctx, "telegram: remove tag failed", "client_id", fl.tags.clientID, "tag", tag, "err", err)
		b.answerCallback(ctx, cb, "Не вдалося прибрати мітку")
		return
	}
	b.answerCallback(ctx, cb, "Прибрано")
	if cb.Message == nil {
		return
	}
	b.refreshTagsMenu(ctx, cb, fl.tags.clientID, fl.tags.clientName)
}

// remainingTagsByCategory groups the vocabulary's not-yet-applied labels by
// category, in ListTagDefs' order (category, then label) — shared by
// pickShowAddTagPicker (which only needs the category names) and
// pickShowCategoryTags (which needs one category's labels).
// remainingTagsByCategory also returns the vocabulary's total size, so
// callers can tell "nothing left to add because the vocabulary itself is
// empty" apart from "nothing left because this client already has every
// tag" — two very different situations that looked identical before.
func (b *AdminBot) remainingTagsByCategory(ctx context.Context, clientID string) (categories []string, byCategory map[string][]string, totalDefs int, err error) {
	defs, err := b.tags.ListTagDefs(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	current, err := b.tags.Tags(ctx, clientID)
	if err != nil {
		return nil, nil, 0, err
	}
	byCategory = make(map[string][]string)
	for _, d := range defs {
		if slices.Contains(current, d.Label) {
			continue
		}
		if _, ok := byCategory[d.Category]; !ok {
			categories = append(categories, d.Category)
		}
		byCategory[d.Category] = append(byCategory[d.Category], d.Label)
	}
	return categories, byCategory, len(defs), nil
}

// pickShowAddTagPicker handles "➕ Додати тег" — the vocabulary's first
// level: which category. Staff picks, never types, a tag — see
// pickShowCategoryTags for the second level (tags within one category).
func (b *AdminBot) pickShowAddTagPicker(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "tags_menu" || b.tags == nil {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /tags")
		return
	}
	categories, _, totalDefs, err := b.remainingTagsByCategory(ctx, fl.tags.clientID)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: tags: list failed", "client_id", fl.tags.clientID, "err", err)
		b.answerCallback(ctx, cb, "Помилка отримання міток")
		return
	}
	b.answerCallback(ctx, cb, "")
	if cb.Message == nil {
		return
	}
	chatID := cb.Message.Chat.ID

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(categories)+1)
	for _, category := range categories {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(category, "tagcat:"+category),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "‹ Назад"), "tagback"),
	))

	var text string
	switch {
	case totalDefs == 0:
		// Distinct from "already all applied" below — telling staff a
		// client "already has every tag" when none exist at all would be
		// actively misleading about the system's own state.
		text = fmt.Sprintf(b.tr(ctx, chatID, "%s: словник міток ще порожній. Додайте мітки в CRM (⚙ Управління тегами)."), fl.tags.clientName)
	case len(categories) == 0:
		text = fmt.Sprintf(b.tr(ctx, chatID, "%s: усі мітки вже додані."), fl.tags.clientName)
	default:
		text = fmt.Sprintf(b.tr(ctx, chatID, "%s — оберіть категорію:"), fl.tags.clientName)
	}
	edited := tgbotapi.NewEditMessageTextAndMarkup(chatID, cb.Message.MessageID, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit add-tag category picker failed", "err", err)
	}
}

// pickShowCategoryTags is the vocabulary's second level — every
// not-yet-applied tag within the tapped category.
func (b *AdminBot) pickShowCategoryTags(ctx context.Context, cb *tgbotapi.CallbackQuery, category string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "tags_menu" || b.tags == nil {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /tags")
		return
	}
	_, byCategory, _, err := b.remainingTagsByCategory(ctx, fl.tags.clientID)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: tags: list failed", "client_id", fl.tags.clientID, "err", err)
		b.answerCallback(ctx, cb, "Помилка отримання міток")
		return
	}
	b.answerCallback(ctx, cb, "")
	if cb.Message == nil {
		return
	}
	chatID := cb.Message.Chat.ID

	labels := byCategory[category]
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(labels)+1)
	for _, label := range labels {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "tagset:"+label),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "‹ Назад"), "tagcatback"),
	))

	// Client name rendered here too (not just on the main menu) — so
	// staff opening a second /tags session for a different client mid-flow
	// at least has a visible cue if a stale drill-down screen from the
	// first client is still on screen.
	text := fmt.Sprintf(b.tr(ctx, chatID, "%s · «%s» — оберіть мітку:"), fl.tags.clientName, category)
	edited := tgbotapi.NewEditMessageTextAndMarkup(chatID, cb.Message.MessageID, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit category tags picker failed", "err", err)
	}
}

func (b *AdminBot) pickSetTag(ctx context.Context, cb *tgbotapi.CallbackQuery, label string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "tags_menu" || b.tags == nil {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /tags")
		return
	}
	if err := b.tags.AddTag(ctx, fl.tags.clientID, label, strconv.FormatInt(cb.From.ID, 10)); err != nil {
		b.log.ErrorContext(ctx, "telegram: add tag failed", "client_id", fl.tags.clientID, "tag", label, "err", err)
		b.answerCallback(ctx, cb, "Не вдалося додати мітку")
		return
	}
	b.answerCallback(ctx, cb, "Додано")
	if cb.Message == nil {
		return
	}
	b.refreshTagsMenu(ctx, cb, fl.tags.clientID, fl.tags.clientName)
}

// pickTagsBack returns from the category picker to the main tags menu.
func (b *AdminBot) pickTagsBack(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "tags_menu" {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /tags")
		return
	}
	b.answerCallback(ctx, cb, "")
	if cb.Message == nil {
		return
	}
	b.refreshTagsMenu(ctx, cb, fl.tags.clientID, fl.tags.clientName)
}

// pickCategoryBack returns from one category's tag list to the category
// picker — one level up, not all the way to the main menu.
func (b *AdminBot) pickCategoryBack(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	b.pickShowAddTagPicker(ctx, cb)
}

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
		b.answerCallback(ctx, cb, "Скасовано")
		b.editCallbackMessage(ctx, cb, "Скасовано")
	default:
		b.answerCallback(ctx, cb, "")
	}
}

func (b *AdminBot) pickAdvocateForGroup(ctx context.Context, cb *tgbotapi.CallbackQuery, clientID string) {
	if _, err := b.store.FindClient(ctx, clientID); err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb, "Клієнта не знайдено")
		return
	}
	advocates, err := b.store.ListAdvocates(ctx, true)
	if err != nil {
		b.log.ErrorContext(ctx, "telegram: creategroup: list advocates failed", "err", err)
		b.answerCallback(ctx, cb, "Помилка отримання адвокатів")
		return
	}
	if len(advocates) == 0 {
		b.answerCallback(ctx, cb, "Немає активних адвокатів")
		b.editCallbackMessage(ctx, cb, "Немає жодного активного адвоката — спочатку /advocate")
		return
	}
	b.flows[cb.From.ID] = &flow{step: "creategroup_advocate", creategroup: creategroupDraft{clientID: clientID}}
	b.answerCallback(ctx, cb, "")

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(advocates))
	for _, a := range advocates {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(a.FullName, "crgrp:adv:"+a.ID),
		))
	}
	if cb.Message == nil {
		return
	}
	edited := tgbotapi.NewEditMessageTextAndMarkup(cb.Message.Chat.ID, cb.Message.MessageID, b.tr(ctx, cb.Message.Chat.ID, "Оберіть адвоката:"), tgbotapi.NewInlineKeyboardMarkup(rows...))
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit advocate-picker message failed", "err", err)
	}
}

func (b *AdminBot) confirmCreateGroup(ctx context.Context, cb *tgbotapi.CallbackQuery, advocateID string) {
	fl := b.flows[cb.From.ID]
	if fl == nil || fl.step != "creategroup_advocate" {
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /creategroup")
		return
	}
	clientID := fl.creategroup.clientID

	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb, "Клієнта не знайдено")
		return
	}
	advocate, err := b.findAdvocate(ctx, advocateID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find advocate failed", "advocate_id", advocateID, "err", err)
		b.answerCallback(ctx, cb, "Адвоката не знайдено")
		return
	}
	fl.creategroup.advocateID = advocateID
	b.answerCallback(ctx, cb, "")

	if cb.Message == nil {
		return
	}
	chatID := cb.Message.Chat.ID
	text := fmt.Sprintf(b.tr(ctx, chatID, "Створити групу: %s + %s?"), client.Name, advocate.FullName)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "✅ Створити"), "crgrp:confirm"),
			tgbotapi.NewInlineKeyboardButtonData(b.tr(ctx, chatID, "❌ Скасувати"), "crgrp:cancel"),
		),
	)
	edited := tgbotapi.NewEditMessageTextAndMarkup(cb.Message.Chat.ID, cb.Message.MessageID, text, keyboard)
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit group-confirm message failed", "err", err)
	}
}

// scans ListAdvocates instead of a dedicated repo method — roster is small, used once
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
		b.answerCallback(ctx, cb, "Сесія застаріла, почніть з /creategroup")
		return
	}
	clientID, advocateID := fl.creategroup.clientID, fl.creategroup.advocateID
	delete(b.flows, cb.From.ID)

	client, err := b.store.FindClient(ctx, clientID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find client failed", "client_id", clientID, "err", err)
		b.answerCallback(ctx, cb, "Клієнта не знайдено")
		return
	}
	advocate, err := b.findAdvocate(ctx, advocateID)
	if err != nil {
		b.log.WarnContext(ctx, "telegram: creategroup: find advocate failed", "advocate_id", advocateID, "err", err)
		b.answerCallback(ctx, cb, "Адвоката не знайдено")
		return
	}

	clientUsername, ok := publicUsername(client.TelegramName)
	if !ok {
		b.log.WarnContext(ctx, "telegram: creategroup: client has no public username", "client_id", clientID, "telegram_name", client.TelegramName)
		b.answerCallback(ctx, cb, "У клієнта немає публічного юзернейму")
		b.editCallbackMessage(ctx, cb, "Не можу створити групу: у клієнта немає публічного @username в Telegram (лише ім'я/телефон). Попросіть клієнта встановити юзернейм в налаштуваннях Telegram.")
		return
	}
	if advocate.TelegramUsername == "" {
		b.log.WarnContext(ctx, "telegram: creategroup: advocate has no username", "advocate_id", advocate.ID)
		b.answerCallback(ctx, cb, "У адвоката немає юзернейму")
		b.editCallbackMessage(ctx, cb, "Не можу створити групу: у адвоката немає @username.")
		return
	}

	b.log.InfoContext(ctx, "telegram: creategroup: creating group", "client_id", clientID, "client_username", clientUsername, "advocate_username", advocate.TelegramUsername)
	b.answerCallback(ctx, cb, "Створюю групу...")

	title := "Абаліс: " + client.Name
	if _, err := b.groups.CreateGroup(ctx, title, []string{clientUsername, advocate.TelegramUsername}); err != nil {
		b.log.ErrorContext(ctx, "telegram: creategroup: create group failed", "client_id", clientID, "err", err)
		b.editCallbackMessage(ctx, cb, "Не вдалося створити групу. Деталі — в логах сервера.")
		return
	}
	b.log.InfoContext(ctx, "telegram: creategroup: group created", "client_id", clientID)
	b.editCallbackMessage(ctx, cb, fmt.Sprintf(b.tr(ctx, cb.From.ID, "Групу створено ✅ (%s + %s)"), client.Name, advocate.FullName))
}

// ok is false with no public username — MTProto can't invite by display name alone
func publicUsername(telegramName string) (string, bool) {
	return strings.CutPrefix(telegramName, "@")
}

func (b *AdminBot) editCallbackMessage(ctx context.Context, cb *tgbotapi.CallbackQuery, text string) {
	if cb.Message == nil {
		return
	}
	edited := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, b.tr(ctx, cb.Message.Chat.ID, text))
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit callback message failed", "err", err)
	}
}

// use when text has <code>/<b> — editCallbackMessage sends them as literal brackets
func (b *AdminBot) editCallbackMessageHTML(ctx context.Context, cb *tgbotapi.CallbackQuery, text string) {
	if cb.Message == nil {
		return
	}
	edited := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, b.tr(ctx, cb.Message.Chat.ID, text))
	edited.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(edited); err != nil {
		b.log.WarnContext(ctx, "telegram: edit callback message failed", "err", err)
	}
}

func (b *AdminBot) answerCallback(ctx context.Context, cb *tgbotapi.CallbackQuery, text string) {
	text = b.tr(ctx, cb.From.ID, text)
	if _, err := b.bot.Request(tgbotapi.NewCallback(cb.ID, text)); err != nil {
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

// showPrice false = advocate's own card; advocate doesn't bill, just does the work
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

// tr applied per label, not to the whole card — live data means no fixed template to key against
func buildClientInfoCard(client consultations.Client, latest consultations.Consultation, hasConsultation bool, clientCases []cases.Case, adminURL string, tr func(string) string) string {
	telegram := client.TelegramName
	if telegram == "" {
		telegram = "—"
	}

	var b strings.Builder
	fmt.Fprintf(&b, tr("🔎 <b>Картка клієнта</b>\n\nClient ID: <code>%s</code>\nІм'я: %s\nТелефон: %s\nTelegram: %s\nCRM: %s/clients/%s"),
		client.ID, html.EscapeString(client.Name), html.EscapeString(client.Phone), html.EscapeString(telegram),
		adminURL, client.ID,
	)

	b.WriteString(tr("\n\n<b>📅 Консультація</b>\n"))
	if hasConsultation {
		fmt.Fprintf(&b, "ID: <code>%s</code>\n%s %s · %s",
			latest.ID, latest.ScheduledAt.Format("02.01.2006"), latest.ScheduledAt.Format("15:04"), tr(statusLabel(latest.Status)),
		)
	} else {
		b.WriteString(tr("ще не було"))
	}

	fmt.Fprintf(&b, tr("\n\n<b>📁 Справи (%d)</b>"), len(clientCases))
	if len(clientCases) == 0 {
		fmt.Fprintf(&b, tr("\nще нема — /case <code>%s</code>"), client.ID)
	}
	for _, c := range clientCases {
		advocate := c.AdvocateName
		if advocate == "" {
			advocate = "—"
		}
		advocateID := c.AdvocateID
		if advocateID == "" {
			advocateID = "—"
		}
		fmt.Fprintf(&b, tr("\n\n<code>%s</code>\n%s\nАдвокат: %s (<code>%s</code>)\n%s/%s грн · залишок %s\nСтатус: %s"),
			c.ID, html.EscapeString(c.Category), html.EscapeString(advocate), advocateID,
			formatAmount(c.PaidAmount), formatAmount(c.Fee), formatAmount(c.Owed()), tr(caseStatusLabel(c.Status)),
		)
		for _, p := range c.Payments {
			fmt.Fprintf(&b, "\n  · %s — %s грн", p.PaidAt.Format("02.01.2006"), formatAmount(p.Amount))
		}
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
	if _, err := b.bot.Send(tgbotapi.NewMessage(chatID, b.tr(ctx, chatID, text))); err != nil {
		b.log.ErrorContext(ctx, "telegram: admin bot send failed", "err", err)
	}
}

func (b *AdminBot) sendHTML(ctx context.Context, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, b.tr(ctx, chatID, text))
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.bot.Send(msg); err != nil {
		b.log.ErrorContext(ctx, "telegram: admin bot send failed", "err", err)
	}
}

func (b *AdminBot) newMsg(ctx context.Context, chatID int64, text string) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(chatID, b.tr(ctx, chatID, text))
}

// missing keys pass through unchanged — client-facing text is deliberately absent from ukToRu, so that's safe
func (b *AdminBot) tr(ctx context.Context, chatID int64, uk string) string {
	lang, ok := b.staffLangs[chatID]
	if !ok {
		var err error
		lang, err = b.store.GetStaffLanguage(ctx, chatID)
		if err != nil {
			b.log.WarnContext(ctx, "telegram: get staff language failed", "err", err)
			lang = consultations.StaffLangUK
		}
		b.staffLangs[chatID] = lang
	}
	if lang != consultations.StaffLangRU {
		return uk
	}
	if ru, ok := ukToRu[uk]; ok {
		return ru
	}
	return uk
}

// keyed by the untranslated template, Sprintf verbs included — translate before formatting, not after
// excludes client-facing text (offers, intake/invoice, consultation cards) — stays Ukrainian
var ukToRu = map[string]string{
	btnInvoice:     "🧾 Счёт",
	btnConsult:     "📋 Подтверждение записи",
	btnBook:        "📌 Записать клиента",
	btnAdvocateAdd: "👤 Добавить адвоката",
	btnAdvocates:   "👥 Список адвокатов",
	btnCase:        "📁 Завести дело",
	btnPay:         "💰 Оплата по делу",
	btnCaseClose:   "✅ Закрыть дело",
	btnCreateGroup: "👨‍👩‍👧 Создать группу",
	btnClientInfo:  "🔎 Карточка клиента",
	btnEditClient:  "✏️ Редактировать клиента",
	btnIntakeLink:  "📋 Анкета для клиента",
	btnHelp:        "❓ Справка",
	btnLanguage:    "🌐 Язык",

	"Прізвище":    "Фамилия",
	"Ім'я":        "Имя",
	"По батькові": "Отчество",
	"Телефон":     "Телефон",
	"Email":       "Email",

	"Заплановано":  "Запланировано",
	"Провів ✅":     "Провёл ✅",
	"Скасував ❌":   "Отменил ❌",
	"Не прийшов 🚫": "Не пришёл 🚫",
	"В роботі":     "В работе",
	"Виконано ✅":   "Выполнено ✅",
	"Скасовано ❌":  "Отменено ❌",

	helpText: `📖 <b>Справка по командам бота</b>

Типовой сценарий: клиент обратился → найти/завести его → записать на консультацию или сразу завести дело → фиксировать оплаты → закрыть дело.

<b>👤 Клиенты</b>
/client &lt;имя/телефон/ID&gt; — найти клиента: карточка со всеми ID (клиент/консультация/дела), история оплат, ссылка на CRM.
/edit — исправить имя/телефон/email клиента пошагово, кнопками.
/tags — метки клиента вручную: добавить из готового списка или убрать (отдельно от автоматических — должник/риск неявки и т.п.).
/book &lt;имя/телефон/ID&gt; — записать на консультацию. Клиента нет в базе — бот сам предложит завести нового.

<b>📁 Дела</b>
/case &lt;имя/телефон/ID&gt; — завести дело (ходатайство/иск/сопровождение). Так же заводит нового клиента, если нужно.
/pay — записать оплату по делу: Case ID → сумма → дата (сегодня или вручную). Или одной строкой: /pay &lt;Case ID&gt; &lt;сумма&gt; — тогда дата = сегодня.
/caseclose &lt;Case ID&gt; — отметить дело выполненным.

<b>👨‍⚖️ Адвокаты</b>
/advocate &lt;ФИО&gt; — добавить в ростер, даёт одноразовую ссылку для адвоката (перешлите ему).
/advocates — список активных, можно деактивировать (старые дела не исчезнут).

<b>💳 Другое</b>
/invoice &lt;сумма&gt; — счёт на оплату с номером карты, для клиента.
/consult — текст-подтверждение записи на консультацию (с условиями оферты), для клиента.
/creategroup — Telegram-группа клиент+адвокат (нужен связанный личный аккаунт).
/intakelink — универсальная ссылка на анкету для нового клиента (без бронирования конкретной даты) — скиньте в смс/мессенджер.

Команда без аргументов сама спросит, что нужно, шаг за шагом — не обязательно помнить точный синтаксис.

/menu — кнопки вместо команд.
/help — эта справка.`,

	"Готово. Справа <code>%s</code> (%s, %s), сума %s грн.\n\nЩоб додати оплату: /pay <code>%s</code> &lt;сума&gt;\nЩоб позначити виконаною: /caseclose <code>%s</code>": "Готово. Дело <code>%s</code> (%s, %s), сумма %s грн.\n\nЧтобы добавить оплату: /pay <code>%s</code> &lt;сумма&gt;\nЧтобы отметить выполненным: /caseclose <code>%s</code>",

	"📅 Сьогодні":  "📅 Сегодня",
	"🗓 Інша дата": "🗓 Другая дата",
	"Оплата +%s грн (%s). Всього оплачено: %s з %s грн. Залишок: %s грн.":                                              "Оплата +%s грн (%s). Всего оплачено: %s из %s грн. Остаток: %s грн.",
	"Формат: /pay &lt;Case ID&gt; &lt;сума&gt;, наприклад /pay <code>3f0b8beb-23c2-4ad4-90fb-48064c9359d4</code> 5000": "Формат: /pay &lt;Case ID&gt; &lt;сумма&gt;, например /pay <code>3f0b8beb-23c2-4ad4-90fb-48064c9359d4</code> 5000",

	"Адвоката збережено. Перешліть йому одноразове посилання:":                        "Адвокат сохранён. Перешлите ему одноразовую ссылку:",
	"Активні адвокати (тап — деактивувати):":                                          "Активные адвокаты (тап — деактивировать):",
	"Немає активних адвокатів — спочатку /advocate <ПІБ>, потім заново /case.":        "Нет активных адвокатов — сначала /advocate <ФИО>, потом заново /case.",
	"Немає активних адвокатів — спочатку /advocate <ПІБ>, потім заново /creategroup.": "Нет активных адвокатов — сначала /advocate <ФИО>, потом заново /creategroup.",
	"Не вдалося зберегти справу, спробуйте ще раз.":                                   "Не удалось сохранить дело, попробуйте ещё раз.",
	"Не вдалося зберегти адвоката, спробуйте ще раз.":                                 "Не удалось сохранить адвоката, попробуйте ещё раз.",
	"Не вдалося отримати список адвокатів.":                                           "Не удалось получить список адвокатов.",
	"Активних адвокатів немає. Додати: /advocate <ПІБ>":                               "Активных адвокатов нет. Добавить: /advocate <ФИО>",
	"Помилка отримання адвокатів, спробуйте ще раз.":                                  "Ошибка получения адвокатов, попробуйте ещё раз.",
	"Оберіть адвоката, який веде справу:":                                             "Выберите адвоката, который ведёт дело:",
	"Оберіть адвоката:":                                     "Выберите адвоката:",
	"Адвоката не знайдено":                                  "Адвоката не найдено",
	"Немає активних адвокатів":                              "Нет активных адвокатов",
	"У адвоката немає юзернейму":                            "У адвоката нет юзернейма",
	"Не можу створити групу: у адвоката немає @username.":   "Не могу создать группу: у адвоката нет @username.",
	"Немає жодного активного адвоката — спочатку /advocate": "Нет ни одного активного адвоката — сначала /advocate",
	"Деактивовано":                                          "Деактивировано",
	"Не вдалося деактивувати":                               "Не удалось деактивировать",
	"Деактивовано ✅ (старі справи не змінились)":            "Деактивировано ✅ (старые дела не изменились)",

	"Кілька збігів, оберіть клієнта:":                                      "Несколько совпадений, выберите клиента:",
	"Клієнта не знайдено. Створити нового на ім'я «%s»? Що є з контактів?": "Клиента не найдено. Создать нового на имя «%s»? Что есть из контактов?",
	"Клієнта не знайдено. Телефон %s. Введіть ім'я нового клієнта:":        "Клиента не найдено. Телефон %s. Введите имя нового клиента:",
	"📞 Телефон":           "📞 Телефон",
	"✉️ Email":            "✉️ Email",
	"💬 Telegram":          "💬 Telegram",
	"⏭ Нічого немає":      "⏭ Ничего нет",
	"Клієнта не знайдено": "Клиента не найдено",
	"Клієнт: %s (%s). Введіть дату консультації (наприклад 29.07.2026):": "Клиент: %s (%s). Введите дату консультации (например 29.07.2026):",
	"Клієнт: %s (%s). Введіть суму договору в грн (наприклад 15000):":    "Клиент: %s (%s). Введите сумму договора в грн (например 15000):",
	"Не вдалося створити клієнта, спробуйте ще раз.":                     "Не удалось создать клиента, попробуйте ещё раз.",

	"Це майбутній запис чи консультація вже відбулася (заводите заднім числом)?": "Это будущая запись или консультация уже состоялась (заводите задним числом)?",
	"📅 Майбутня":                 "📅 Будущая",
	"✅ Вже відбулася, оплачена":  "✅ Уже состоялась, оплачена",
	"Готово. Перешліть клієнту:": "Готово. Перешлите клиенту:",
	"Додано заднім числом (%s). За бажанням перешліть клієнту:\n\n%s": "Добавлено задним числом (%s). При желании перешлите клиенту:\n\n%s",
	"✅ Провів, оплатив":    "✅ Провёл, оплатил",
	"⏳ Провів, не оплатив": "⏳ Провёл, не оплатил",
	"Провів, оплатив ✅":    "Провёл, оплатил ✅",
	"Провів, не оплатив ⏳": "Провёл, не оплатил ⏳",
	"✅ Провів":             "✅ Провёл",
	"❌ Скасував":           "❌ Отменил",
	"🚫 Не прийшов":         "🚫 Не пришёл",
	"Статус: %s":           "Статус: %s",
	"\n\nСтатус: %s":       "\n\nСтатус: %s",
	"Не розпізнав дату/час. Спробуйте ще раз: /book <code>%s</code>": "Не распознал дату/время. Попробуйте ещё раз: /book <code>%s</code>",
	"Не вдалося зберегти консультацію, спробуйте ще раз.":            "Не удалось сохранить консультацию, попробуйте ещё раз.",

	"Оберіть клієнта:":                     "Выберите клиента:",
	"Створити групу: %s + %s?":             "Создать группу: %s + %s?",
	"✅ Створити":                           "✅ Создать",
	"❌ Скасувати":                          "❌ Отменить",
	"Скасовано":                            "Отменено",
	"Групу створено ✅ (%s + %s)":           "Группа создана ✅ (%s + %s)",
	"Створюю групу...":                     "Создаю группу...",
	"У клієнта немає публічного юзернейму": "У клиента нет публичного юзернейма",
	"Не можу створити групу: у клієнта немає публічного @username в Telegram (лише ім'я/телефон). Попросіть клієнта встановити юзернейм в налаштуваннях Telegram.": "Не могу создать группу: у клиента нет публичного @username в Telegram (только имя/телефон). Попросите клиента установить юзернейм в настройках Telegram.",
	"Не вдалося створити групу. Деталі — в логах сервера.":               "Не удалось создать группу. Детали — в логах сервера.",
	"Нічого не знайдено. Спробуйте /creategroup ще раз з іншим запитом.": "Ничего не найдено. Попробуйте /creategroup ещё раз с другим запросом.",

	"Помилка отримання справ.":                                      "Ошибка получения дел.",
	"Помилка отримання справ":                                       "Ошибка получения дел",
	"Нічого не знайдено. Спробуйте /client ще раз з іншим запитом.": "Ничего не найдено. Попробуйте /client ещё раз с другим запросом.",
	"Нічого не знайдено. Спробуйте /edit ще раз з іншим запитом.":   "Ничего не найдено. Попробуйте /edit ещё раз с другим запросом.",
	"Введіть нове значення для «%s»:":                               "Введите новое значение для «%s»:",
	"Не вдалося оновити дані клієнта, спробуйте ще раз.":            "Не удалось обновить данные клиента, попробуйте ещё раз.",
	"Оновлено ✅":                             "Обновлено ✅",
	"Введіть телефон:":                       "Введите телефон:",
	"Введіть email:":                         "Введите email:",
	"Введіть Telegram (@username або ім'я):": "Введите Telegram (@username или имя):",
	"Клієнта створюю без контактів…":         "Создаю клиента без контактов…",

	"🔎 <b>Картка клієнта</b>\n\nClient ID: <code>%s</code>\nІм'я: %s\nТелефон: %s\nTelegram: %s\nCRM: %s/clients/%s": "🔎 <b>Карточка клиента</b>\n\nClient ID: <code>%s</code>\nИмя: %s\nТелефон: %s\nTelegram: %s\nCRM: %s/clients/%s",
	"\n\n<b>📅 Консультація</b>\n":       "\n\n<b>📅 Консультация</b>\n",
	"ще не було":                        "ещё не было",
	"\n\n<b>📁 Справи (%d)</b>":          "\n\n<b>📁 Дела (%d)</b>",
	"\nще нема — /case <code>%s</code>": "\nещё нет — /case <code>%s</code>",
	"\n\n<code>%s</code>\n%s\nАдвокат: %s (<code>%s</code>)\n%s/%s грн · залишок %s\nСтатус: %s": "\n\n<code>%s</code>\n%s\nАдвокат: %s (<code>%s</code>)\n%s/%s грн · остаток %s\nСтатус: %s",

	"Введіть суму в грн (наприклад 1500):": "Введите сумму в грн (например 1500):",
	"Введіть ПІБ клієнта:":                 "Введите ФИО клиента:",
	"Введіть Client ID, ім'я або телефон клієнта (якщо не знайдеться — запропоную завести нового):": "Введите Client ID, имя или телефон клиента (если не найдётся — предложу завести нового):",
	"Введіть ПІБ адвоката:":                                                      "Введите ФИО адвоката:",
	"Введіть Case ID:":                                                           "Введите Case ID:",
	"Введіть ім'я, юзернейм або телефон клієнта:":                                "Введите имя, юзернейм или телефон клиента:",
	"Введіть ім'я, телефон або Client ID клієнта:":                               "Введите имя, телефон или Client ID клиента:",
	"Не розпізнав суму. Введіть число, наприклад 1500.":                          "Не распознал сумму. Введите число, например 1500.",
	"Не розпізнав дату. Введіть у форматі 19.08.2026:":                           "Не распознал дату. Введите в формате 19.08.2026:",
	"Введіть дату консультації (наприклад 29.07.2026):":                          "Введите дату консультации (например 29.07.2026):",
	"Введіть час консультації (наприклад 15:00):":                                "Введите время консультации (например 15:00):",
	"Введіть вартість консультації в грн (наприклад 800):":                       "Введите стоимость консультации в грн (например 800):",
	"Не розпізнав суму. Введіть число, наприклад 800.":                           "Не распознал сумму. Введите число, например 800.",
	"Введіть справу/коментар (наприклад: справа про мікрокредити, треба позов):": "Введите дело/комментарий (например: дело о микрокредитах, нужен иск):",
	"Не розпізнав суму. Введіть число, наприклад 15000.":                         "Не распознал сумму. Введите число, например 15000.",
	"Напрямок справи (%s) — введіть один з варіантів або свій:":                  "Направление дела (%s) — введите один из вариантов или свой:",
	"Не розпізнав суму.":                                                         "Не распознал сумму.",
	"Формат: /caseclose <Case ID>":                                               "Формат: /caseclose <Case ID>",
	"Не вдалося оновити справу — перевірте Case ID.":                             "Не удалось обновить дело — проверьте Case ID.",
	"Справу позначено виконаною ✅":                                               "Дело отмечено выполненным ✅",
	"Не вдалося зберегти оплату — перевірте Case ID.":                            "Не удалось сохранить оплату — проверьте Case ID.",
	"ПІБ не може бути порожнім.":                                                 "ФИО не может быть пустым.",
	"Помилка пошуку, спробуйте ще раз.":                                          "Ошибка поиска, попробуйте ещё раз.",
	"Коли внесена оплата?":                                                       "Когда внесена оплата?",
	"Введіть дату (наприклад 19.08.2026):":                                       "Введите дату (например 19.08.2026):",
	"Оплата за сьогодні…":                                                        "Оплата за сегодня…",
	"Сесія застаріла, почніть з /book":                                           "Сессия устарела, начните с /book",
	"Сесія застаріла, почніть з /case":                                           "Сессия устарела, начните с /case",
	"Сесія застаріла, почніть з /pay":                                            "Сессия устарела, начните с /pay",
	"Сесія застаріла, почніть з /creategroup":                                    "Сессия устарела, начните с /creategroup",
	"Сесія застаріла, почніть заново":                                            "Сессия устарела, начните заново",
	"Не вдалося зберегти":                                                        "Не удалось сохранить",

	"Меню:": "Меню:",
	"Це посилання для клієнта — перешліть його, не переходьте самі.": "Эта ссылка для клиента — перешлите её, не переходите сами.",
	"Що змінити?": "Что изменить?",

	btnTags: "🏷 Метки клиента",
	"Нічого не знайдено. Спробуйте /tags ще раз з іншим запитом.": "Ничего не найдено. Попробуйте /tags ещё раз с другим запросом.",
	"Мітки клієнта недоступні.":                                   "Метки клиента недоступны.",
	"Помилка отримання міток, спробуйте ще раз.":                  "Ошибка получения меток, попробуйте ещё раз.",
	"➕ Додати тег":                     "➕ Добавить метку",
	"🏷 Мітки клієнта: %s":              "🏷 Метки клиента: %s",
	"Міток ще немає.":                  "Меток ещё нет.",
	"Сесія застаріла, почніть з /tags": "Сессия устарела, начните с /tags",
	"Не вдалося прибрати мітку":        "Не удалось убрать метку",
	"Прибрано":                         "Убрано",
	"Додано":                           "Добавлено",
	"‹ Назад":                          "‹ Назад",
	"Оберіть мітку:":                   "Выберите метку:",
	"%s: словник міток ще порожній. Додайте мітки в CRM (⚙ Управління тегами).": "%s: словарь меток ещё пуст. Добавьте метки в CRM (⚙ Управление тегами).",
	"%s: усі мітки вже додані.":      "%s: все метки уже добавлены.",
	"%s — оберіть категорію:":        "%s — выберите категорию:",
	"%s · «%s» — оберіть мітку:":     "%s · «%s» — выберите метку:",
	"Помилка отримання списку міток": "Ошибка получения списка меток",
	"Помилка отримання міток":        "Ошибка получения меток",
	"Не вдалося додати мітку":        "Не удалось добавить метку",
}

// inverse of ukToRu — lets handle() map a Russian-rendered button back to Ukrainian
var ruToUk = func() map[string]string {
	m := make(map[string]string, len(ukToRu))
	for uk, ru := range ukToRu {
		m[ru] = uk
	}
	return m
}()

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

func looksLikePhone(s string) bool {
	digits, other := 0, 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+' || r == '-' || r == '(' || r == ')' || r == ' ':
		default:
			other++
		}
	}
	return digits >= 6 && other == 0
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
