package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

func TestRiteCadence(t *testing.T) {
	cases := map[string]*model.Recurrence{
		"daily":         {Every: model.RecurDaily, Interval: 1},
		"weekday":       {Every: model.RecurWeekly, Weekdays: []time.Weekday{0, 1, 2, 3, 4}},
		"weekly":        {Every: model.RecurWeekly, Interval: 1},
		"biweekly":      {Every: model.RecurWeekly, Interval: 2},
		"every 3 days":  {Every: model.RecurDaily, Interval: 3},
		"every 3 weeks": {Every: model.RecurWeekly, Interval: 3},
	}
	for want, r := range cases {
		if got := RiteCadence(r); got != want {
			t.Errorf("RiteCadence = %q, want %q", got, want)
		}
	}
}

func TestMusterWhen(t *testing.T) {
	now := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC) // Wed
	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }

	if l, over := MusterWhen(day(16), now); l != "today" || over {
		t.Errorf("today: got %q overdue=%v", l, over)
	}
	if l, over := MusterWhen(day(17), now); l != "tomorrow" || over {
		t.Errorf("tomorrow: got %q overdue=%v", l, over)
	}
	if l, over := MusterWhen(day(15), now); l != "overdue" || !over {
		t.Errorf("overdue: got %q overdue=%v", l, over)
	}
	if l, _ := MusterWhen(day(21), now); l != "Mon Sep 21" {
		t.Errorf("far day: got %q, want 'Mon Sep 21'", l)
	}
}

// The badge is empty for an unscheduled quest and carries the cadence + muster
// for a rite.
func TestQuestScheduleBadge(t *testing.T) {
	Init(true)
	now := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)

	if b := QuestScheduleBadge(&model.Quest{}, now); b != "" {
		t.Errorf("unscheduled quest should have no badge, got %q", b)
	}
	muster := now
	b := QuestScheduleBadge(&model.Quest{Muster: &muster, Recurrence: &model.Recurrence{Every: model.RecurDaily}}, now)
	// Styling wraps the words but leaves them intact, so match on the raw output.
	if !strings.Contains(b, "daily") || !strings.Contains(b, "today") {
		t.Errorf("rite badge should show cadence + muster, got %q", b)
	}
}

// A finished task with a past muster shows the date it was set for, never
// "overdue" — done work can't be late.
func TestQuestScheduleBadgeDoneShowsDate(t *testing.T) {
	Init(true)
	now := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC) // Wed
	past := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC) // Sat, before now

	done := &model.Quest{Muster: &past, Status: model.StatusDone}
	b := QuestScheduleBadge(done, now)
	if strings.Contains(b, "overdue") {
		t.Errorf("a done task must not read 'overdue', got %q", b)
	}
	if !strings.Contains(b, "Sat Sep 12") {
		t.Errorf("a done task should show its muster date, got %q", b)
	}

	// An open task with the same past muster still reads overdue.
	open := &model.Quest{Muster: &past}
	if b := QuestScheduleBadge(open, now); !strings.Contains(b, "overdue") {
		t.Errorf("an open past-muster task should read overdue, got %q", b)
	}
}
