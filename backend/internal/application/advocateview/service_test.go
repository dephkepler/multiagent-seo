package advocateview_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	app "multiagent-seo/internal/application/advocateview"
	domain "multiagent-seo/internal/domain/advocateview"
)

// fakeRepo records the Owner it was asked with — the point of these tests is
// that the scope reaching the SQL is the logged-in advocate and nothing else.
type fakeRepo struct {
	advocates map[string]domain.Advocate
	lastOwner domain.Owner
	noteText  string
	status    string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{advocates: map[string]domain.Advocate{
		"borzov": {ID: "borzov", FullName: "Ярослав Борзов", IsActive: true, CommissionPercent: 35},
	}}
}

func (r *fakeRepo) Advocate(_ context.Context, id string) (domain.Advocate, error) {
	a, ok := r.advocates[id]
	if !ok {
		return domain.Advocate{}, domain.ErrNotFound
	}
	return a, nil
}

func (r *fakeRepo) ListCases(_ context.Context, owner domain.Owner) ([]domain.Case, error) {
	r.lastOwner = owner
	return []domain.Case{{ID: "case-1", Fee: 1000, Paid: 250}}, nil
}

func (r *fakeRepo) ListClients(_ context.Context, owner domain.Owner) ([]domain.Client, error) {
	r.lastOwner = owner
	return nil, nil
}

func (r *fakeRepo) GetClient(_ context.Context, owner domain.Owner, _ string) (domain.Card, error) {
	r.lastOwner = owner
	return domain.Card{}, nil
}

func (r *fakeRepo) AddNote(_ context.Context, owner domain.Owner, _, text, _ string) (domain.Note, error) {
	r.lastOwner = owner
	r.noteText = text
	return domain.Note{ID: "note-1", Text: text}, nil
}

func (r *fakeRepo) UpdateCaseStatus(_ context.Context, owner domain.Owner, _, status string) error {
	r.lastOwner = owner
	r.status = status
	return nil
}

func (r *fakeRepo) CollectionsByMonth(_ context.Context, owner domain.Owner) ([]domain.MonthMoney, error) {
	r.lastOwner = owner
	return []domain.MonthMoney{{Month: "2026-05", Collected: 10000}}, nil
}

func (r *fakeRepo) PaidOut(context.Context, string) (float64, error) {
	return 1000, nil
}

func TestReadsAreScopedToTheLoggedInAdvocate(t *testing.T) {
	repo := newFakeRepo()
	svc := app.NewService(repo)

	if _, err := svc.Cases(context.Background(), "borzov"); err != nil {
		t.Fatalf("cases: %v", err)
	}
	want := domain.Owner{ID: "borzov", FullName: "Ярослав Борзов"}
	if repo.lastOwner != want {
		t.Errorf("owner = %+v, want %+v", repo.lastOwner, want)
	}
}

// An empty advocate id is the dangerous case: it is what an admin login or a
// half-created account produces, and a query scoped to nothing must never be
// allowed to become a query scoped to everything.
func TestEmptyAdvocateIsRefusedBeforeAnyQuery(t *testing.T) {
	repo := newFakeRepo()
	svc := app.NewService(repo)

	for _, id := range []string{"", "   "} {
		if _, err := svc.Cases(context.Background(), id); !errors.Is(err, domain.ErrNoAdvocate) {
			t.Errorf("cases(%q) error = %v, want ErrNoAdvocate", id, err)
		}
	}
	if repo.lastOwner != (domain.Owner{}) {
		t.Errorf("the repository was queried anyway with %+v", repo.lastOwner)
	}
}

func TestUnknownAdvocateIsNotFound(t *testing.T) {
	svc := app.NewService(newFakeRepo())

	if _, err := svc.Cases(context.Background(), "stranger"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestAddNoteTrimsAndRejectsEmpty(t *testing.T) {
	repo := newFakeRepo()
	svc := app.NewService(repo)

	if _, err := svc.AddNote(context.Background(), "borzov", "client-1", "   ", "user-1"); !errors.Is(err, domain.ErrEmptyNote) {
		t.Errorf("blank note error = %v, want ErrEmptyNote", err)
	}
	if _, err := svc.AddNote(context.Background(), "borzov", "client-1", strings.Repeat("я", 4001), "user-1"); !errors.Is(err, domain.ErrNoteTooLong) {
		t.Errorf("oversized note error = %v, want ErrNoteTooLong", err)
	}

	if _, err := svc.AddNote(context.Background(), "borzov", "client-1", "  подзвонив клієнту  ", "user-1"); err != nil {
		t.Fatalf("add note: %v", err)
	}
	if repo.noteText != "подзвонив клієнту" {
		t.Errorf("stored note = %q, want it trimmed", repo.noteText)
	}
}

func TestSetCaseStatusRefusesCancel(t *testing.T) {
	repo := newFakeRepo()
	svc := app.NewService(repo)

	if err := svc.SetCaseStatus(context.Background(), "borzov", "case-1", "cancelled"); !errors.Is(err, domain.ErrStatusNotAllowed) {
		t.Errorf("cancel error = %v, want ErrStatusNotAllowed", err)
	}
	if err := svc.SetCaseStatus(context.Background(), "borzov", "case-1", "nonsense"); !errors.Is(err, domain.ErrStatusNotAllowed) {
		t.Errorf("garbage status error = %v, want ErrStatusNotAllowed", err)
	}
	if repo.status != "" {
		t.Errorf("a refused status reached the repository: %q", repo.status)
	}

	if err := svc.SetCaseStatus(context.Background(), "borzov", "case-1", "completed"); err != nil {
		t.Fatalf("completed: %v", err)
	}
	if repo.status != "completed" {
		t.Errorf("stored status = %q, want completed", repo.status)
	}
}

func TestSettlementIsOwnMoneyOnly(t *testing.T) {
	svc := app.NewService(newFakeRepo())

	got, err := svc.Settlement(context.Background(), "borzov")
	if err != nil {
		t.Fatalf("settlement: %v", err)
	}
	if got.AdvocateID != "borzov" || got.Collected != 10000 || got.Accrued != 3500 || got.Outstanding != 2500 {
		t.Errorf("settlement = %+v, want own 10000/3500/2500", got)
	}
}

func TestStatsPricesMonthsAtTheOwnRate(t *testing.T) {
	svc := app.NewService(newFakeRepo())

	got, err := svc.Stats(context.Background(), "borzov")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(got.Months) != 1 || got.Months[0].Accrued != 3500 {
		t.Errorf("months = %+v, want 2026-05 accrued at 35%%", got.Months)
	}
	if got.Cases != 1 || got.ClientDebt != 750 {
		t.Errorf("stats = %+v, want 1 case and 750 owed", got)
	}
}
