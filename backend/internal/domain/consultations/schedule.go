package consultations

import (
	"context"
	"slices"
	"time"
)

// WeekdaysMonToFri is the firm's working week.
var WeekdaysMonToFri = []time.Weekday{
	time.Monday,
	time.Tuesday,
	time.Wednesday,
	time.Thursday,
	time.Friday,
}

// Schedule is the grid of times the firm offers for consultations. It holds no
// state: the picker generates from it, so moving the working day is a config
// change and nothing else.
type Schedule struct {
	// Location is the firm's own timezone, and the grid is laid out in it
	// rather than in UTC. Kyiv moves to summer time, so a day built by adding
	// 24-hour steps to an instant slides by an hour on the changeover.
	Location *time.Location
	Weekdays []time.Weekday
	// Open and Close are offsets from midnight: 10h to 18h is a 10:00–18:00
	// day. Close bounds the last slot's end, not its start.
	Open  time.Duration
	Close time.Duration
	Slot  time.Duration
	// LeadTime is the notice a client has to give. Without it the picker offers
	// an hour starting in ten minutes, which nobody on either side can keep.
	LeadTime time.Duration
	// Horizon is how far ahead slots are offered at all.
	Horizon time.Duration
}

// Availability is the one thing the picker needs from storage. Declared apart
// from Store on purpose: that interface has two dozen methods and every fake in
// the bot's tests implements it, so widening it would cost each of them a method
// they never call.
type Availability interface {
	// HeldSlots returns the starts already taken in [from, to), in any order.
	HeldSlots(ctx context.Context, from, to time.Time) ([]time.Time, error)
}

// FreeSlots lists the starts a client may pick, in chronological order: the
// grid between now+LeadTime and now+Horizon, minus what held already covers.
func (s Schedule) FreeSlots(now time.Time, held []time.Time) []time.Time {
	if s.Slot <= 0 || s.Close-s.Open < s.Slot || len(s.Weekdays) == 0 {
		return nil
	}

	location := s.Location
	if location == nil {
		location = time.UTC
	}

	// Keyed by instant, so a held slot stored in any zone still matches the one
	// generated here.
	taken := make(map[int64]struct{}, len(held))
	for _, slot := range held {
		taken[slot.Unix()] = struct{}{}
	}

	earliest := now.Add(s.LeadTime)
	latest := now.Add(s.Horizon)

	local := now.In(location)
	firstDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)

	var free []time.Time
	// AddDate rather than Add: it counts calendar days, which is what survives a
	// daylight-saving change.
	for day := firstDay; !day.After(latest); day = day.AddDate(0, 0, 1) {
		if !s.opensOn(day.Weekday()) {
			continue
		}
		for offset := s.Open; offset+s.Slot <= s.Close; offset += s.Slot {
			// Built from wall-clock parts, not by adding an offset to midnight:
			// on a changeover day the two disagree by an hour, and only this
			// one lands on the time the firm actually means.
			start := time.Date(
				day.Year(),
				day.Month(),
				day.Day(),
				int(offset/time.Hour),
				int(offset%time.Hour/time.Minute),
				0,
				0,
				location,
			)
			if start.Before(earliest) {
				continue
			}
			if start.After(latest) {
				return free
			}
			if _, busy := taken[start.Unix()]; busy {
				continue
			}
			free = append(free, start)
		}
	}
	return free
}

// Offers reports whether start is a slot this schedule would hand out right
// now. The client sends back an instant, not an index, so without this a
// request could name 03:00 on a Sunday, an hour already taken, or a date a year
// out — the picker's grid would be advisory and the calendar would not.
func (s Schedule) Offers(now, start time.Time, held []time.Time) bool {
	return slices.ContainsFunc(s.FreeSlots(now, held), start.Equal)
}

func (s Schedule) opensOn(day time.Weekday) bool {
	return slices.Contains(s.Weekdays, day)
}
