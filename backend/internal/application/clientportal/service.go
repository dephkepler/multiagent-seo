// Package clientportal is what a client can do for themselves: see the free
// hours, ask for one, and look at what they have asked for.
//
// It orchestrates pieces that already exist — the schedule, the client list,
// the lead pipeline — and adds one rule of its own: nothing here ever takes a
// client id from the caller. The id comes from whoever the launch signature
// resolved to, so a request cannot name somebody else.
package clientportal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"multiagent-seo/internal/domain/cases"
	"multiagent-seo/internal/domain/consultations"
	"multiagent-seo/internal/domain/webleads"
)

var (
	ErrNoName  = errors.New("clientportal: name is required")
	ErrNoPhone = errors.New("clientportal: phone is required")
	// ErrSlotNotOffered means the instant is not one the picker would hand out:
	// off the grid, outside the working day, inside the lead time, past the
	// horizon, or already held.
	ErrSlotNotOffered = errors.New("clientportal: slot is not on offer")
)

// Clients is the slice of the client store this service needs.
type Clients interface {
	CreateClient(ctx context.Context, name, phone, email, telegramName string) (consultations.Client, error)
	SetClientEmail(ctx context.Context, clientID, email string) error
	SetClientTelegram(ctx context.Context, clientID string, chatID int64, telegramName string) error
	FindClient(ctx context.Context, clientID string) (consultations.Client, error)
}

// Consultations is the slice of the consultation store this service needs.
type Consultations interface {
	consultations.Availability
	HoldSlot(ctx context.Context, clientID string, at time.Time, createdBy string) (consultations.Consultation, error)
	ConsultationsOf(ctx context.Context, clientID string) ([]consultations.Consultation, error)
}

// Leads is the existing intake pipeline: one submission notifies staff, writes
// the lead and the client, and mirrors the row into the spreadsheet. Reused
// rather than re-implemented so a request from the Mini App lands in exactly
// the same places as one from the bot or the website form.
type Leads interface {
	SubmitLead(ctx context.Context, lead webleads.Lead) (string, error)
}

type Service struct {
	schedule consultations.Schedule
	consults Consultations
	clients  Clients
	leads    Leads
	log      *slog.Logger
	// now is a field so the slot arithmetic is testable without waiting for a
	// Tuesday.
	now func() time.Time
}

type Deps struct {
	Schedule      consultations.Schedule
	Consultations Consultations
	Clients       Clients
	Leads         Leads
	Log           *slog.Logger
}

func NewService(deps Deps) *Service {
	return &Service{
		schedule: deps.Schedule,
		consults: deps.Consultations,
		clients:  deps.Clients,
		leads:    deps.Leads,
		log:      deps.Log,
		now:      time.Now,
	}
}

// BookingOptions is everything the booking form has to render.
type BookingOptions struct {
	Slots []time.Time
	// Categories is the practice-area list, served rather than duplicated in
	// the app: a copy there would drift from the one staff pick from.
	Categories []string
}

// BookingOptions draws the grid and names the practice areas.
func (s *Service) BookingOptions(ctx context.Context) (BookingOptions, error) {
	now := s.now()
	held, err := s.consults.HeldSlots(ctx, now, now.Add(s.schedule.Horizon))
	if err != nil {
		return BookingOptions{}, fmt.Errorf("booking options: %w", err)
	}
	return BookingOptions{
		Slots:      s.schedule.FreeSlots(now, held),
		Categories: cases.Categories,
	}, nil
}

// Request is one submission from the Mini App: the client's own details, what
// they need, and optionally the hour they picked.
type Request struct {
	Name     string
	Phone    string
	Email    string
	Category string
	Question string
	// Slot is zero when the client only wants to be called back.
	Slot time.Time
	// TelegramID and TelegramName come from the verified launch, never from the
	// request body.
	TelegramID   int64
	TelegramName string
}

// Submission is what came of a Request.
type Submission struct {
	ClientID string
	// Consultation is set only when a slot was asked for and held.
	Consultation *consultations.Consultation
}

// Submit records the request: it holds the slot first, then files the lead.
//
// That order is deliberate. The slot is the part that can fail for a reason the
// client can act on — somebody took it a second earlier — and failing after the
// lead was filed would leave staff a request whose time nobody agreed to.
func (s *Service) Submit(ctx context.Context, req Request) (Submission, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Phone = strings.TrimSpace(req.Phone)
	if req.Name == "" {
		return Submission{}, ErrNoName
	}
	phone := webleads.NormalizePhone(req.Phone)
	if phone == "" {
		return Submission{}, ErrNoPhone
	}

	if !req.Slot.IsZero() {
		if err := s.checkSlot(ctx, req.Slot); err != nil {
			return Submission{}, err
		}
	}

	// Find-or-create by phone, which is how every other intake path resolves a
	// client too: a returning client typing the same number lands on the row
	// they already have instead of a duplicate.
	client, err := s.clients.CreateClient(ctx, req.Name, phone, req.Email, req.TelegramName)
	if err != nil {
		return Submission{}, fmt.Errorf("submit request: resolve client: %w", err)
	}

	// Binding before anything else can fail: this is what lets the client open
	// the app again and be recognised, and what reminders are sent to.
	if req.TelegramID != 0 {
		if err := s.clients.SetClientTelegram(ctx, client.ID, req.TelegramID, req.TelegramName); err != nil {
			return Submission{}, fmt.Errorf("submit request: bind telegram: %w", err)
		}
	}

	out := Submission{ClientID: client.ID}
	if !req.Slot.IsZero() {
		consultation, err := s.consults.HoldSlot(ctx, client.ID, req.Slot, createdByClient)
		if errors.Is(err, consultations.ErrSlotTaken) {
			return Submission{}, ErrSlotNotOffered
		}
		if err != nil {
			return Submission{}, fmt.Errorf("submit request: hold slot: %w", err)
		}
		out.Consultation = &consultation
	}

	if _, err := s.leads.SubmitLead(ctx, s.lead(req, phone)); err != nil {
		// The slot is held and the client exists, so the request is real; only
		// the notification and the spreadsheet row are missing. Reporting a
		// failure here would send the client into submitting all over again and
		// lose them the hour they just took.
		s.log.ErrorContext(ctx, "clientportal: lead submission failed after the slot was held",
			"client_id", client.ID, "err", err)
	}

	if req.Email != "" {
		// SubmitLead resolves name and phone; email is not on a Lead at all.
		if err := s.clients.SetClientEmail(ctx, client.ID, req.Email); err != nil {
			s.log.WarnContext(ctx, "clientportal: set client email failed", "client_id", client.ID, "err", err)
		}
	}
	return out, nil
}

// createdBy marks who booked, and the CRM shows it next to the consultation.
const createdByClient = "client"

func (s *Service) checkSlot(ctx context.Context, slot time.Time) error {
	now := s.now()
	held, err := s.consults.HeldSlots(ctx, now, now.Add(s.schedule.Horizon))
	if err != nil {
		return fmt.Errorf("submit request: held slots: %w", err)
	}
	if !s.schedule.Offers(now, slot, held) {
		return ErrSlotNotOffered
	}
	return nil
}

func (s *Service) lead(req Request, phone string) webleads.Lead {
	message := req.Question
	if req.Category != "" {
		message = fmt.Sprintf("Категорія: %s\n\n%s", req.Category, message)
	}
	if !req.Slot.IsZero() {
		// Kept in the text for the staff notification, which is read as prose.
		// The machine-readable version is the consultation this held.
		message = fmt.Sprintf("Обраний час: %s\n\n%s", s.formatSlot(req.Slot), message)
	}

	return webleads.Lead{
		// Same shape the bot's own intake uses, so nothing downstream has to
		// learn a second format.
		MessageID:  fmt.Sprintf("tma-%d-%d", req.TelegramID, s.now().UnixNano()),
		ReceivedAt: s.now(),
		Name:       req.Name,
		Phone:      phone,
		Message:    message,
		Page:       leadPage(!req.Slot.IsZero()),
	}
}

func (s *Service) formatSlot(slot time.Time) string {
	location := s.schedule.Location
	if location == nil {
		location = time.UTC
	}
	return slot.In(location).Format("02.01.2006 15:04")
}

func leadPage(booked bool) string {
	if booked {
		return "Telegram-застосунок"
	}
	return "Telegram-застосунок: анкета"
}

// Profile is the client's own view of themselves.
type Profile struct {
	Name  string
	Phone string
	// NotificationsOn reports whether reminders can reach this client, which is
	// true exactly when their chat is bound.
	NotificationsOn bool
	Consultations   []consultations.Consultation
}

// Me reads the caller's own record. clientID comes from the principal.
func (s *Service) Me(ctx context.Context, clientID string) (Profile, error) {
	if clientID == "" {
		return Profile{}, ErrNoClient
	}
	client, err := s.clients.FindClient(ctx, clientID)
	if err != nil {
		return Profile{}, fmt.Errorf("client profile %q: %w", clientID, err)
	}
	list, err := s.consults.ConsultationsOf(ctx, clientID)
	if err != nil {
		return Profile{}, fmt.Errorf("client profile %q: %w", clientID, err)
	}
	return Profile{
		Name:            client.Name,
		Phone:           client.Phone,
		NotificationsOn: client.TelegramChatID != 0,
		Consultations:   list,
	}, nil
}

// ErrNoClient guards the one thing that must never fall back to "no filter": a
// caller with no client id asking for a client's data.
var ErrNoClient = errors.New("clientportal: no client in this request")
