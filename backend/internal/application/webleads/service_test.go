package webleads_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appleads "multiagent-seo/internal/application/webleads"
	domain "multiagent-seo/internal/domain/webleads"
)

type fakeMail struct {
	messages    []domain.Message
	fetchErr    error
	markSeenErr error
	seen        []uint32
}

func (f *fakeMail) FetchUnseen(context.Context) ([]domain.Message, error) {
	return f.messages, f.fetchErr
}

func (f *fakeMail) MarkSeen(_ context.Context, uid uint32) error {
	if f.markSeenErr != nil {
		return f.markSeenErr
	}
	f.seen = append(f.seen, uid)
	return nil
}

type fakeNotifier struct {
	sendErr error
	sent    []string
}

func (f *fakeNotifier) SendMessage(_ context.Context, text string) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, text)
	return nil
}

type fakeStore struct {
	saveErr           error
	resolveClientErr  error
	markSyncedErr     error
	saved             []domain.Lead
	sheetSyncedForIDs []string
}

// ResolveClient fakes the real repository's phone -> client matching: a
// deterministic ClientID derived from the phone, so tests can assert on it
// without a database.
func (f *fakeStore) ResolveClient(_ context.Context, phone, _ string) (string, error) {
	if f.resolveClientErr != nil {
		return "", f.resolveClientErr
	}
	if phone == "" {
		return "", nil
	}
	return "client-" + phone, nil
}

func (f *fakeStore) Save(_ context.Context, lead domain.Lead) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, lead)
	return nil
}

func (f *fakeStore) MarkSheetSynced(_ context.Context, messageID string) error {
	if f.markSyncedErr != nil {
		return f.markSyncedErr
	}
	f.sheetSyncedForIDs = append(f.sheetSyncedForIDs, messageID)
	return nil
}

type fakeSheet struct {
	appendErr error
	appended  []domain.Lead
}

func (f *fakeSheet) AppendRow(_ context.Context, lead domain.Lead) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.appended = append(f.appended, lead)
	return nil
}

func TestProcessNewLeads_SendsSavesSyncsSheetAndMarksSeen(t *testing.T) {
	mail := &fakeMail{messages: []domain.Message{
		{UID: 1, MessageID: "<a@abalis.com.ua>", From: "info@abalis.com.ua", Subject: "Заявка", Body: "Имя: Анна\nТелефон: 0971234567", Date: time.Now()},
	}}
	notifier := &fakeNotifier{}
	store := &fakeStore{}
	sheet := &fakeSheet{}

	appleads.NewService(mail, notifier, store, sheet, nil).ProcessNewLeads(context.Background())

	if len(notifier.sent) != 1 {
		t.Fatalf("sent = %d messages, want 1", len(notifier.sent))
	}
	if len(store.saved) != 1 || store.saved[0].Name != "Анна" {
		t.Fatalf("saved = %v, want one lead named Анна", store.saved)
	}
	if len(sheet.appended) != 1 {
		t.Fatalf("appended = %v, want one row", sheet.appended)
	}
	if sheet.appended[0].ClientID == "" {
		t.Fatalf("appended[0].ClientID is empty, want it resolved before send and carried through to the sheet row")
	}
	if !strings.Contains(notifier.sent[0], "Client ID: <code>client-+380971234567</code>") {
		t.Fatalf("telegram text = %q, want it to contain the resolved Client ID", notifier.sent[0])
	}
	if len(store.sheetSyncedForIDs) != 1 || store.sheetSyncedForIDs[0] != "<a@abalis.com.ua>" {
		t.Fatalf("sheetSyncedForIDs = %v, want [<a@abalis.com.ua>]", store.sheetSyncedForIDs)
	}
	if len(mail.seen) != 1 || mail.seen[0] != 1 {
		t.Fatalf("seen = %v, want [1]", mail.seen)
	}
}

func TestProcessNewLeads_SendFailureLeavesMessageUnseenAndUnsaved(t *testing.T) {
	mail := &fakeMail{messages: []domain.Message{
		{UID: 1, MessageID: "<a@abalis.com.ua>", Body: "Имя: Анна"},
	}}
	notifier := &fakeNotifier{sendErr: errors.New("telegram unavailable")}
	store := &fakeStore{}
	sheet := &fakeSheet{}

	appleads.NewService(mail, notifier, store, sheet, nil).ProcessNewLeads(context.Background())

	if len(mail.seen) != 0 {
		t.Fatalf("seen = %v, want none marked after a failed send (should retry next poll)", mail.seen)
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved = %v, want none saved after a failed send", store.saved)
	}
	if len(sheet.appended) != 0 {
		t.Fatalf("appended = %v, want none appended after a failed send", sheet.appended)
	}
}

func TestProcessNewLeads_SaveFailureStillMarksSeenAndSkipsSheet(t *testing.T) {
	mail := &fakeMail{messages: []domain.Message{
		{UID: 1, MessageID: "<a@abalis.com.ua>", Body: "Имя: Анна"},
	}}
	notifier := &fakeNotifier{}
	store := &fakeStore{saveErr: errors.New("db down")}
	sheet := &fakeSheet{}

	appleads.NewService(mail, notifier, store, sheet, nil).ProcessNewLeads(context.Background())

	if len(mail.seen) != 1 {
		t.Fatalf("seen = %v, want [1] — Telegram already succeeded, a DB hiccup shouldn't cause a resend", mail.seen)
	}
	if len(sheet.appended) != 0 {
		t.Fatalf("appended = %v, want none — sheet_synced_at bookkeeping needs the DB row to exist first", sheet.appended)
	}
}

func TestProcessNewLeads_SheetFailureStillMarksSeen(t *testing.T) {
	mail := &fakeMail{messages: []domain.Message{
		{UID: 1, MessageID: "<a@abalis.com.ua>", Body: "Имя: Анна"},
	}}
	notifier := &fakeNotifier{}
	store := &fakeStore{}
	sheet := &fakeSheet{appendErr: errors.New("sheets api down")}

	appleads.NewService(mail, notifier, store, sheet, nil).ProcessNewLeads(context.Background())

	if len(mail.seen) != 1 {
		t.Fatalf("seen = %v, want [1] — a sheet hiccup shouldn't block Telegram dedup", mail.seen)
	}
	if len(store.sheetSyncedForIDs) != 0 {
		t.Fatalf("sheetSyncedForIDs = %v, want none — append failed, nothing to mark synced", store.sheetSyncedForIDs)
	}
}

func TestProcessNewLeads_FetchFailureIsANoop(t *testing.T) {
	mail := &fakeMail{fetchErr: errors.New("imap down")}
	notifier := &fakeNotifier{}
	store := &fakeStore{}
	sheet := &fakeSheet{}

	appleads.NewService(mail, notifier, store, sheet, nil).ProcessNewLeads(context.Background())

	if len(notifier.sent) != 0 {
		t.Fatalf("sent = %v, want none when fetch fails", notifier.sent)
	}
}
