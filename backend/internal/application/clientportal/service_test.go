package clientportal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"multiagent-seo/internal/domain/cases"
	"multiagent-seo/internal/domain/consultations"
	"multiagent-seo/internal/domain/webleads"
)

type fakeClients struct {
	created   consultations.Client
	bound     map[string]int64
	emails    map[string]string
	findErr   error
	found     consultations.Client
	createErr error
	bindErr   error
}

func newFakeClients() *fakeClients {
	return &fakeClients{
		created: consultations.Client{ID: "client-1", Name: "Петро Коваль", Phone: "+380501112233"},
		bound:   map[string]int64{},
		emails:  map[string]string{},
	}
}

func (f *fakeClients) CreateClient(_ context.Context, name, phone, _, _ string) (consultations.Client, error) {
	if f.createErr != nil {
		return consultations.Client{}, f.createErr
	}
	f.created.Name = name
	f.created.Phone = phone
	return f.created, nil
}

func (f *fakeClients) SetClientEmail(_ context.Context, clientID, email string) error {
	f.emails[clientID] = email
	return nil
}

func (f *fakeClients) SetClientTelegram(_ context.Context, clientID string, chatID int64, _ string) error {
	if f.bindErr != nil {
		return f.bindErr
	}
	f.bound[clientID] = chatID
	return nil
}

func (f *fakeClients) FindClient(_ context.Context, _ string) (consultations.Client, error) {
	if f.findErr != nil {
		return consultations.Client{}, f.findErr
	}
	return f.found, nil
}

type fakeConsultations struct {
	held    []time.Time
	holdErr error
	holds   []time.Time
	list    []consultations.Consultation
	heldErr error
	listErr error
}

func (f *fakeConsultations) HeldSlots(_ context.Context, _, _ time.Time) ([]time.Time, error) {
	if f.heldErr != nil {
		return nil, f.heldErr
	}
	return f.held, nil
}

func (f *fakeConsultations) HoldSlot(_ context.Context, clientID string, at time.Time, createdBy string) (consultations.Consultation, error) {
	if f.holdErr != nil {
		return consultations.Consultation{}, f.holdErr
	}
	f.holds = append(f.holds, at)
	return consultations.Consultation{
		ID:          "consultation-1",
		ClientID:    clientID,
		ScheduledAt: at,
		Status:      consultations.StatusRequested,
		CreatedBy:   createdBy,
	}, nil
}

func (f *fakeConsultations) ConsultationsOf(_ context.Context, _ string) ([]consultations.Consultation, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

type fakeLeads struct {
	submitted []webleads.Lead
	err       error
}

func (f *fakeLeads) SubmitLead(_ context.Context, lead webleads.Lead) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.submitted = append(f.submitted, lead)
	return "client-1", nil
}

// A Monday at 07:00 Kyiv, before the working day, so the whole grid is ahead.
func testNow(t *testing.T) (time.Time, *time.Location) {
	t.Helper()
	location, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatalf("load Europe/Kyiv: %v", err)
	}
	return time.Date(2026, time.August, 24, 7, 0, 0, 0, location), location
}

func newTestService(t *testing.T, clients *fakeClients, consults *fakeConsultations, leads *fakeLeads) *Service {
	t.Helper()
	now, location := testNow(t)
	svc := NewService(Deps{
		Schedule: consultations.Schedule{
			Location: location,
			Weekdays: consultations.WeekdaysMonToFri,
			Open:     10 * time.Hour,
			Close:    18 * time.Hour,
			Slot:     time.Hour,
			LeadTime: 2 * time.Hour,
			Horizon:  14 * 24 * time.Hour,
		},
		Consultations: consults,
		Clients:       clients,
		Leads:         leads,
		Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	svc.now = func() time.Time { return now }
	return svc
}

func TestSubmitHoldsTheSlotAndFilesTheLead(t *testing.T) {
	_, location := testNow(t)
	clients, consults, leads := newFakeClients(), &fakeConsultations{}, &fakeLeads{}
	svc := newTestService(t, clients, consults, leads)

	slot := time.Date(2026, time.August, 24, 11, 0, 0, 0, location)
	got, err := svc.Submit(context.Background(), Request{
		Name:         "Петро Коваль",
		Phone:        "0501112233",
		Email:        "petro@example.com",
		Category:     "Спадщина",
		Question:     "Потрібна консультація",
		Slot:         slot,
		TelegramID:   42,
		TelegramName: "@petro",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if got.Consultation == nil {
		t.Fatal("no consultation came back")
	}
	if got.Consultation.Status != consultations.StatusRequested {
		t.Errorf("status = %q, want requested", got.Consultation.Status)
	}
	if len(consults.holds) != 1 || !consults.holds[0].Equal(slot) {
		t.Errorf("held %v, want one hold at %s", consults.holds, slot)
	}
	// Binding the chat is what lets this client be recognised on their next
	// launch, and what reminders are sent to.
	if clients.bound["client-1"] != 42 {
		t.Errorf("chat bound to %d, want 42", clients.bound["client-1"])
	}
	if len(leads.submitted) != 1 {
		t.Fatalf("filed %d leads, want 1", len(leads.submitted))
	}
	if leads.submitted[0].Phone != "+380501112233" {
		t.Errorf("lead phone = %q, want the normalised +380501112233", leads.submitted[0].Phone)
	}
	if clients.emails["client-1"] != "petro@example.com" {
		t.Errorf("email = %q, want petro@example.com", clients.emails["client-1"])
	}
}

// The client sends an instant, so the grid has to be enforced server-side or it
// is only a suggestion.
func TestSubmitRefusesASlotOffTheGrid(t *testing.T) {
	_, location := testNow(t)

	for name, slot := range map[string]time.Time{
		"before opening":   time.Date(2026, time.August, 24, 9, 0, 0, 0, location),
		"after closing":    time.Date(2026, time.August, 24, 19, 0, 0, 0, location),
		"off the hour":     time.Date(2026, time.August, 24, 11, 30, 0, 0, location),
		"on a Sunday":      time.Date(2026, time.August, 30, 11, 0, 0, 0, location),
		"inside lead time": time.Date(2026, time.August, 24, 8, 0, 0, 0, location),
		"past the horizon": time.Date(2026, time.October, 5, 11, 0, 0, 0, location),
		"in the past":      time.Date(2026, time.August, 21, 11, 0, 0, 0, location),
	} {
		t.Run(name, func(t *testing.T) {
			clients, consults, leads := newFakeClients(), &fakeConsultations{}, &fakeLeads{}
			svc := newTestService(t, clients, consults, leads)

			_, err := svc.Submit(context.Background(), Request{
				Name: "Петро", Phone: "0501112233", Slot: slot, TelegramID: 42,
			})
			if !errors.Is(err, ErrSlotNotOffered) {
				t.Fatalf("err = %v, want ErrSlotNotOffered", err)
			}
			// Nothing may be written for a request that was refused.
			if len(consults.holds) != 0 || len(leads.submitted) != 0 || len(clients.bound) != 0 {
				t.Errorf("a refused request still wrote something: holds=%v leads=%d bound=%v",
					consults.holds, len(leads.submitted), clients.bound)
			}
		})
	}
}

func TestSubmitRefusesAnAlreadyHeldSlot(t *testing.T) {
	_, location := testNow(t)
	slot := time.Date(2026, time.August, 24, 11, 0, 0, 0, location)

	clients, leads := newFakeClients(), &fakeLeads{}
	consults := &fakeConsultations{held: []time.Time{slot}}
	svc := newTestService(t, clients, consults, leads)

	_, err := svc.Submit(context.Background(), Request{
		Name: "Петро", Phone: "0501112233", Slot: slot, TelegramID: 42,
	})
	if !errors.Is(err, ErrSlotNotOffered) {
		t.Fatalf("err = %v, want ErrSlotNotOffered", err)
	}
}

// Somebody took the hour between the grid being drawn and this request landing.
// The storage-level guard is the one that catches it, and it has to read as the
// same refusal to the app.
func TestSubmitTranslatesALostRaceIntoTheSameRefusal(t *testing.T) {
	_, location := testNow(t)
	clients, leads := newFakeClients(), &fakeLeads{}
	consults := &fakeConsultations{holdErr: consultations.ErrSlotTaken}
	svc := newTestService(t, clients, consults, leads)

	_, err := svc.Submit(context.Background(), Request{
		Name:  "Петро",
		Phone: "0501112233",
		Slot:  time.Date(2026, time.August, 24, 11, 0, 0, 0, location),
	})
	if !errors.Is(err, ErrSlotNotOffered) {
		t.Fatalf("err = %v, want ErrSlotNotOffered", err)
	}
}

func TestSubmitWithoutASlotIsACallBackRequest(t *testing.T) {
	clients, consults, leads := newFakeClients(), &fakeConsultations{}, &fakeLeads{}
	svc := newTestService(t, clients, consults, leads)

	got, err := svc.Submit(context.Background(), Request{
		Name: "Петро", Phone: "0501112233", TelegramID: 42,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got.Consultation != nil {
		t.Errorf("held a slot for a request that named none: %+v", got.Consultation)
	}
	if len(leads.submitted) != 1 {
		t.Fatalf("filed %d leads, want 1", len(leads.submitted))
	}
	if clients.bound["client-1"] != 42 {
		t.Error("a call-back request must still bind the chat")
	}
}

func TestSubmitRequiresANameAndAPhone(t *testing.T) {
	for name, req := range map[string]Request{
		"no name":        {Phone: "0501112233"},
		"blank name":     {Name: "   ", Phone: "0501112233"},
		"no phone":       {Name: "Петро"},
		"unusable phone": {Name: "Петро", Phone: "   "},
	} {
		t.Run(name, func(t *testing.T) {
			clients, consults, leads := newFakeClients(), &fakeConsultations{}, &fakeLeads{}
			svc := newTestService(t, clients, consults, leads)

			if _, err := svc.Submit(context.Background(), req); err == nil {
				t.Fatal("err = nil, want a validation error")
			}
			if len(leads.submitted) != 0 {
				t.Error("filed a lead for an invalid request")
			}
		})
	}
}

// The slot is held and the client exists, so the request is real: telling them
// it failed would send them round again and cost them the hour they just took.
func TestSubmitSurvivesAFailedLeadSubmission(t *testing.T) {
	_, location := testNow(t)
	clients, consults := newFakeClients(), &fakeConsultations{}
	leads := &fakeLeads{err: errors.New("telegram is down")}
	svc := newTestService(t, clients, consults, leads)

	got, err := svc.Submit(context.Background(), Request{
		Name:  "Петро",
		Phone: "0501112233",
		Slot:  time.Date(2026, time.August, 24, 11, 0, 0, 0, location),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got.Consultation == nil {
		t.Error("the slot was held but not reported back")
	}
}

func TestBookingOptionsExcludeWhatIsHeldAndNameThePracticeAreas(t *testing.T) {
	_, location := testNow(t)
	taken := time.Date(2026, time.August, 24, 12, 0, 0, 0, location)
	svc := newTestService(t, newFakeClients(), &fakeConsultations{held: []time.Time{taken}}, &fakeLeads{})

	options, err := svc.BookingOptions(context.Background())
	if err != nil {
		t.Fatalf("BookingOptions: %v", err)
	}
	if len(options.Slots) == 0 {
		t.Fatal("no slots at all")
	}
	for _, slot := range options.Slots {
		if slot.Equal(taken) {
			t.Errorf("offered the held slot %s", slot)
		}
	}
	// Served from the domain so the form cannot drift from what staff pick from.
	if len(options.Categories) != len(cases.Categories) {
		t.Errorf("got %d categories, want the %d in cases.Categories", len(options.Categories), len(cases.Categories))
	}
}

func TestMeReportsWhetherRemindersCanReachTheClient(t *testing.T) {
	for name, tc := range map[string]struct {
		chatID int64
		want   bool
	}{
		"chat bound":     {chatID: 42, want: true},
		"chat not bound": {chatID: 0, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			clients := newFakeClients()
			clients.found = consultations.Client{
				ID:             "client-1",
				Name:           "Петро Коваль",
				Phone:          "+380501112233",
				TelegramChatID: tc.chatID,
			}
			consults := &fakeConsultations{list: []consultations.Consultation{
				{ID: "c-1", Status: consultations.StatusRequested},
			}}
			svc := newTestService(t, clients, consults, &fakeLeads{})

			profile, err := svc.Me(context.Background(), "client-1")
			if err != nil {
				t.Fatalf("Me: %v", err)
			}
			if profile.NotificationsOn != tc.want {
				t.Errorf("NotificationsOn = %v, want %v", profile.NotificationsOn, tc.want)
			}
			if len(profile.Consultations) != 1 {
				t.Errorf("got %d consultations, want 1", len(profile.Consultations))
			}
		})
	}
}

// A caller with no client id must never fall through to an unfiltered read.
func TestMeRefusesACallerWithNoClient(t *testing.T) {
	svc := newTestService(t, newFakeClients(), &fakeConsultations{}, &fakeLeads{})

	if _, err := svc.Me(context.Background(), ""); !errors.Is(err, ErrNoClient) {
		t.Fatalf("err = %v, want ErrNoClient", err)
	}
}
