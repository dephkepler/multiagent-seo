package consultations

import (
	"testing"
	"time"
)

func kyiv(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatalf("load Europe/Kyiv: %v", err)
	}
	return location
}

func workingWeek(location *time.Location) Schedule {
	return Schedule{
		Location: location,
		Weekdays: WeekdaysMonToFri,
		Open:     10 * time.Hour,
		Close:    18 * time.Hour,
		Slot:     time.Hour,
		LeadTime: 2 * time.Hour,
		Horizon:  14 * 24 * time.Hour,
	}
}

func localTimes(slots []time.Time, location *time.Location) []string {
	out := make([]string, 0, len(slots))
	for _, slot := range slots {
		out = append(out, slot.In(location).Format("2006-01-02 15:04"))
	}
	return out
}

func TestFreeSlotsFillsTheWorkingDay(t *testing.T) {
	location := kyiv(t)
	// A Monday, before the day opens.
	now := time.Date(2026, time.August, 24, 7, 0, 0, 0, location)

	slots := workingWeek(location).FreeSlots(now, nil)

	var monday []string
	for _, slot := range localTimes(slots, location) {
		if slot[:10] == "2026-08-24" {
			monday = append(monday, slot[11:])
		}
	}
	want := []string{"10:00", "11:00", "12:00", "13:00", "14:00", "15:00", "16:00", "17:00"}
	if len(monday) != len(want) {
		t.Fatalf("Monday has %d slots (%v), want %d", len(monday), monday, len(want))
	}
	for i, hour := range want {
		if monday[i] != hour {
			t.Errorf("slot %d = %s, want %s", i, monday[i], hour)
		}
	}
}

// 18:00 closes the day, so the last hour offered starts at 17:00.
func TestFreeSlotsNeverRunPastClosing(t *testing.T) {
	location := kyiv(t)
	now := time.Date(2026, time.August, 24, 7, 0, 0, 0, location)

	for _, slot := range workingWeek(location).FreeSlots(now, nil) {
		local := slot.In(location)
		minutes := time.Duration(local.Hour())*time.Hour + time.Duration(local.Minute())*time.Minute
		if minutes < 10*time.Hour || minutes+time.Hour > 18*time.Hour {
			t.Fatalf("slot outside 10:00–18:00: %s", local.Format(time.RFC3339))
		}
	}
}

func TestFreeSlotsSkipTheWeekend(t *testing.T) {
	location := kyiv(t)
	// A Friday afternoon, so the next days generated are Saturday and Sunday.
	now := time.Date(2026, time.August, 28, 16, 0, 0, 0, location)

	for _, slot := range workingWeek(location).FreeSlots(now, nil) {
		switch day := slot.In(location).Weekday(); day {
		case time.Saturday, time.Sunday:
			t.Fatalf("offered %s on a %s", slot.In(location).Format(time.RFC3339), day)
		}
	}
}

// The whole point of LeadTime: a client must not be offered an hour that starts
// before they could plausibly get there.
func TestFreeSlotsRespectTheLeadTime(t *testing.T) {
	location := kyiv(t)
	now := time.Date(2026, time.August, 24, 11, 30, 0, 0, location)

	slots := workingWeek(location).FreeSlots(now, nil)
	if len(slots) == 0 {
		t.Fatal("no slots at all")
	}
	if got := slots[0].In(location).Format("2006-01-02 15:04"); got != "2026-08-24 14:00" {
		t.Errorf("first slot = %s, want 2026-08-24 14:00 (11:30 + 2h lands inside 13:00, so 13:00 is gone)", got)
	}
}

func TestFreeSlotsStopAtTheHorizon(t *testing.T) {
	location := kyiv(t)
	now := time.Date(2026, time.August, 24, 7, 0, 0, 0, location)

	schedule := workingWeek(location)
	schedule.Horizon = 3 * 24 * time.Hour
	slots := schedule.FreeSlots(now, nil)

	if len(slots) == 0 {
		t.Fatal("no slots at all")
	}
	last := slots[len(slots)-1]
	if last.After(now.Add(schedule.Horizon)) {
		t.Errorf("last slot %s is past the horizon %s", last, now.Add(schedule.Horizon))
	}
	// Mon 24th, Tue 25th, Wed 26th at three days out — 8 slots each, and the
	// 24th loses nothing because 07:00 + 2h is still before it opens.
	if len(slots) != 24 {
		t.Errorf("got %d slots, want 24 (%v)", len(slots), localTimes(slots, location))
	}
}

func TestFreeSlotsDropWhatIsAlreadyHeld(t *testing.T) {
	location := kyiv(t)
	now := time.Date(2026, time.August, 24, 7, 0, 0, 0, location)

	held := []time.Time{
		time.Date(2026, time.August, 24, 12, 0, 0, 0, location),
		// Same instant expressed in UTC: storage hands these back in whatever
		// zone the driver picks, and the match must not depend on it.
		time.Date(2026, time.August, 24, 12, 0, 0, 0, location).UTC(),
		time.Date(2026, time.August, 24, 15, 0, 0, 0, location),
	}

	for _, slot := range localTimes(workingWeek(location).FreeSlots(now, held), location) {
		if slot == "2026-08-24 12:00" || slot == "2026-08-24 15:00" {
			t.Errorf("offered a held slot: %s", slot)
		}
	}
}

// Kyiv moves to summer time overnight on 29 March 2026: 03:00 local never
// exists. A grid laid out by adding 24-hour steps drifts by an hour from here
// on, and every slot after the changeover would be offered at the wrong time.
func TestFreeSlotsSurviveTheDaylightSavingChange(t *testing.T) {
	location := kyiv(t)
	now := time.Date(2026, time.March, 27, 7, 0, 0, 0, location)

	schedule := workingWeek(location)
	schedule.Horizon = 5 * 24 * time.Hour
	slots := schedule.FreeSlots(now, nil)

	var monday []string
	for _, slot := range localTimes(slots, location) {
		if slot[:10] == "2026-03-30" {
			monday = append(monday, slot[11:])
		}
	}
	if len(monday) == 0 {
		t.Fatal("no slots on the Monday after the changeover")
	}
	if monday[0] != "10:00" {
		t.Errorf("first slot after the changeover = %s, want 10:00", monday[0])
	}
}

func TestFreeSlotsRefuseAnUnusableSchedule(t *testing.T) {
	location := kyiv(t)
	now := time.Date(2026, time.August, 24, 7, 0, 0, 0, location)

	for name, broken := range map[string]func(Schedule) Schedule{
		"no slot length": func(s Schedule) Schedule { s.Slot = 0; return s },
		"closes at open": func(s Schedule) Schedule { s.Close = s.Open; return s },
		"day shorter than one slot": func(s Schedule) Schedule {
			s.Close = s.Open + 30*time.Minute
			return s
		},
		"no working days": func(s Schedule) Schedule { s.Weekdays = nil; return s },
	} {
		t.Run(name, func(t *testing.T) {
			if slots := broken(workingWeek(location)).FreeSlots(now, nil); slots != nil {
				t.Errorf("got %d slots, want none", len(slots))
			}
		})
	}
}
