package finance

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func rule(id string, day int, mutate func(*Rule)) Rule {
	r := Rule{
		ID:            id,
		Name:          "hosting",
		CategoryCode:  "website",
		Amount:        642,
		DayOfMonth:    day,
		ActiveFrom:    date(2024, time.June, 1),
		IsActive:      true,
		PaymentMethod: PaymentCard,
	}
	if mutate != nil {
		mutate(&r)
	}
	return r
}

func TestPlanRecurringSkipsDaysNotYetDue(t *testing.T) {
	month := date(2026, time.August, 1)
	now := date(2026, time.August, 10)

	got := PlanRecurring([]Rule{rule("a", 5, nil), rule("b", 14, nil)}, month, now)

	if len(got) != 1 || got[0].Rule.ID != "a" {
		t.Fatalf("want only rule a, got %+v", got)
	}
	if want := "rule:a:2026-08"; got[0].ExternalRef != want {
		t.Errorf("ref = %q, want %q", got[0].ExternalRef, want)
	}
	if !got[0].SpentAt.Equal(date(2026, time.August, 5)) {
		t.Errorf("spentAt = %v, want 2026-08-05", got[0].SpentAt)
	}
}

func TestPlanRecurringDueToday(t *testing.T) {
	month := date(2026, time.August, 1)
	got := PlanRecurring([]Rule{rule("a", 10, nil)}, month, date(2026, time.August, 10))
	if len(got) != 1 {
		t.Fatalf("a rule due today must be planned, got %+v", got)
	}
}

func TestPlanRecurringHonoursActiveWindow(t *testing.T) {
	month := date(2026, time.August, 1)
	now := date(2026, time.August, 28)

	rules := []Rule{
		rule("inactive", 5, func(r *Rule) { r.IsActive = false }),
		rule("not-started", 5, func(r *Rule) { r.ActiveFrom = date(2026, time.September, 1) }),
		rule("ended", 5, func(r *Rule) {
			end := date(2026, time.July, 31)
			r.ActiveTo = &end
		}),
		rule("ends-later-this-month", 5, func(r *Rule) {
			end := date(2026, time.August, 20)
			r.ActiveTo = &end
		}),
	}

	got := PlanRecurring(rules, month, now)
	if len(got) != 1 || got[0].Rule.ID != "ends-later-this-month" {
		t.Fatalf("want only ends-later-this-month, got %+v", got)
	}
}

func TestPlanRecurringAutoPostSkipsDraft(t *testing.T) {
	month := date(2026, time.August, 1)
	now := date(2026, time.August, 28)

	got := PlanRecurring([]Rule{
		rule("draft", 5, nil),
		rule("auto", 6, func(r *Rule) { r.AutoPost = true }),
	}, month, now)

	if len(got) != 2 {
		t.Fatalf("want 2 planned, got %d", len(got))
	}
	if got[0].Status != StatusDraft {
		t.Errorf("status = %q, want draft", got[0].Status)
	}
	if got[1].Status != StatusPosted {
		t.Errorf("status = %q, want posted", got[1].Status)
	}
}

func TestPlanAdvocatePayouts(t *testing.T) {
	month := date(2026, time.August, 1)
	now := date(2026, time.September, 3)

	got := PlanAdvocatePayouts([]AdvocateCollection{
		{AdvocateID: "borzov", AdvocateName: "Борзов", Collected: 45000, CommissionPercent: 35},
		{AdvocateID: "no-rate", AdvocateName: "Попов", Collected: 12000},
		{AdvocateID: "no-money", AdvocateName: "Бондар", CommissionPercent: 35},
	}, month, now)

	if len(got) != 1 {
		t.Fatalf("want 1 payout, got %+v", got)
	}
	p := got[0]
	if p.Amount != 15750 {
		t.Errorf("amount = %v, want 15750", p.Amount)
	}
	if want := "advocate:borzov:2026-08"; p.ExternalRef != want {
		t.Errorf("ref = %q, want %q", p.ExternalRef, want)
	}
	if !p.SpentAt.Equal(date(2026, time.August, 31)) {
		t.Errorf("spentAt = %v, want end of month", p.SpentAt)
	}
}

func TestPlanAdvocatePayoutsRoundsToKopecks(t *testing.T) {
	got := PlanAdvocatePayouts([]AdvocateCollection{
		{AdvocateID: "a", Collected: 1000, CommissionPercent: 33.333},
	}, date(2026, time.February, 1), date(2026, time.March, 1))

	if len(got) != 1 || got[0].Amount != 333.33 {
		t.Fatalf("amount = %+v, want 333.33", got)
	}
	if !got[0].SpentAt.Equal(date(2026, time.February, 28)) {
		t.Errorf("spentAt = %v, want 2026-02-28", got[0].SpentAt)
	}
}

func TestPlanAdvocatePayoutsSkipsAnOpenMonth(t *testing.T) {
	month := date(2026, time.August, 1)
	collections := []AdvocateCollection{{AdvocateID: "borzov", Collected: 5000, CommissionPercent: 35}}

	if got := PlanAdvocatePayouts(collections, month, date(2026, time.August, 20)); len(got) != 0 {
		t.Fatalf("mid-month must plan nothing, got %+v", got)
	}
	if got := PlanAdvocatePayouts(collections, month, date(2026, time.August, 31)); len(got) != 0 {
		t.Fatalf("last day of the month is still open, got %+v", got)
	}
	if got := PlanAdvocatePayouts(collections, month, date(2026, time.September, 1)); len(got) != 1 {
		t.Fatalf("month closed, want 1 payout, got %+v", got)
	}
}
