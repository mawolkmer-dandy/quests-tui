package model

import (
	"testing"
	"time"
)

// Recurrence.Next computes the next occurrence date for each rite cadence: daily
// (every N days), plain weekly (every N weeks), and weekly pinned to specific
// weekdays (the next listed weekday). This drives how a rite recycles its muster
// on completion, so an off-by-a-day here would drift every recurring duty.
func TestRecurrenceNext(t *testing.T) {
	// 2026-09-16 is a Wednesday.
	wed := time.Date(2026, 9, 16, 9, 30, 0, 0, time.UTC) // note: not midnight — Next must date-only it

	cases := []struct {
		name string
		rule Recurrence
		want time.Time
	}{
		{"daily", Recurrence{Every: RecurDaily}, date(2026, 9, 17)},
		{"every 3 days", Recurrence{Every: RecurDaily, Interval: 3}, date(2026, 9, 19)},
		{"weekly", Recurrence{Every: RecurWeekly}, date(2026, 9, 23)},
		{"every 2 weeks", Recurrence{Every: RecurWeekly, Interval: 2}, date(2026, 9, 30)},
		{"weekdays from Wed → Thu", Recurrence{Every: RecurWeekly, Weekdays: mondayToFriday()}, date(2026, 9, 17)},
		{"weekly on Monday from Wed → next Mon", Recurrence{Every: RecurWeekly, Weekdays: []time.Weekday{time.Monday}}, date(2026, 9, 21)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.rule.Next(wed)
			if !got.Equal(c.want) {
				t.Fatalf("Next = %s, want %s", got.Format("2006-01-02"), c.want.Format("2006-01-02"))
			}
		})
	}
}

// Called: a quest is called once its muster is today or earlier; overdue counts.
func TestQuestCalled(t *testing.T) {
	today := date(2026, 9, 16)
	tomorrow := date(2026, 9, 17)
	yesterday := date(2026, 9, 15)

	none := Quest{}
	if none.Called(today) {
		t.Error("a quest with no muster is never called")
	}
	future := Quest{Muster: &tomorrow}
	if future.Called(today) {
		t.Error("a quest mustered tomorrow is not called today")
	}
	due := Quest{Muster: &today}
	if !due.Called(today) {
		t.Error("a quest mustered today is called today")
	}
	overdue := Quest{Muster: &yesterday}
	if !overdue.Called(today) {
		t.Error("an overdue muster still counts as called (rolls forward)")
	}
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func mondayToFriday() []time.Weekday {
	return []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
}
