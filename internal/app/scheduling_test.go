package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func scheduleModel(t *testing.T, q model.Quest) *Model {
	ui.Init(true)
	return &Model{
		store:             &store.Store{Quests: []model.Quest{q}},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}
}

// applySchedule writes a one-off muster for the date choices and a rite (with its
// first muster seeded) for the recurring choices, and clears both on "clear".
func TestApplySchedule(t *testing.T) {
	today := model.DateOnly(time.Now())

	t.Run("muster today", func(t *testing.T) {
		m := scheduleModel(t, model.Quest{ID: "q", Title: "Review PR"})
		m.applySchedule("q", "m:today")
		q := m.findQuest("q")
		if q.Muster == nil || !q.Muster.Equal(today) {
			t.Fatalf("muster = %v, want %v", q.Muster, today)
		}
		if q.Recurrence != nil {
			t.Fatalf("a one-off muster must not set a recurrence: %+v", q.Recurrence)
		}
	})

	t.Run("daily rite seeds today's muster", func(t *testing.T) {
		m := scheduleModel(t, model.Quest{ID: "q", Title: "Check emails"})
		m.applySchedule("q", "r:daily")
		q := m.findQuest("q")
		if q.Recurrence == nil || q.Recurrence.Every != model.RecurDaily {
			t.Fatalf("daily rite not set: %+v", q.Recurrence)
		}
		if q.Muster == nil || !q.Muster.Equal(today) {
			t.Fatalf("a daily rite should muster today, got %v", q.Muster)
		}
		if !q.Called(time.Now()) {
			t.Fatal("a rite mustered today should be called")
		}
	})

	t.Run("weekday rite musters on a weekday", func(t *testing.T) {
		m := scheduleModel(t, model.Quest{ID: "q", Title: "Check PRs"})
		m.applySchedule("q", "r:weekdays")
		q := m.findQuest("q")
		if q.Recurrence == nil || len(q.Recurrence.Weekdays) != 5 {
			t.Fatalf("weekday rite not set: %+v", q.Recurrence)
		}
		wd := q.Muster.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("a weekday rite must not muster on a weekend, got %s", wd)
		}
	})

	t.Run("clear removes both", func(t *testing.T) {
		m := scheduleModel(t, model.Quest{ID: "q", Title: "x"})
		m.applySchedule("q", "r:daily")
		m.applySchedule("q", "clear")
		q := m.findQuest("q")
		if q.Muster != nil || q.Recurrence != nil {
			t.Fatalf("clear should wipe schedule: muster=%v rec=%+v", q.Muster, q.Recurrence)
		}
	})
}

// firstMuster starts daily/weekly rites today, and weekday-pinned rites on the
// next qualifying weekday (today when today already qualifies).
func TestFirstMuster(t *testing.T) {
	sat := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC) // a Saturday
	wed := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC) // a Wednesday

	daily := &model.Recurrence{Every: model.RecurDaily}
	if got := firstMuster(daily, sat); !got.Equal(sat) {
		t.Errorf("daily should start today, got %s", got.Format("2006-01-02"))
	}

	weekdays := &model.Recurrence{Every: model.RecurWeekly, Weekdays: weekdaysMonFri()}
	if got := firstMuster(weekdays, sat); got.Weekday() != time.Monday {
		t.Errorf("weekday rite from Sat should start Monday, got %s", got.Weekday())
	}
	if got := firstMuster(weekdays, wed); !got.Equal(wed) {
		t.Errorf("weekday rite from Wed should start same day, got %s", got.Format("2006-01-02"))
	}
}

// The scheduling picker lists a muster group, a rite group, and a trailing clear.
func TestScheduleOptions(t *testing.T) {
	opts := scheduleOptions(time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)) // Wednesday
	musters, rites, clear := 0, 0, 0
	for _, o := range opts {
		switch {
		case o.id == "clear":
			clear++
		case o.id[:2] == "m:":
			musters++
		case o.id[:2] == "r:":
			rites++
		}
	}
	if musters == 0 || rites == 0 || clear != 1 {
		t.Fatalf("want muster + rite groups and one clear, got musters=%d rites=%d clear=%d", musters, rites, clear)
	}
	// The weekly rite names the day it will fall on.
	found := false
	for _, o := range opts {
		if o.id == "r:weekly" && o.label == "Every week (Wed)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("weekly rite should name today's weekday (Wed): %+v", opts)
	}
}
