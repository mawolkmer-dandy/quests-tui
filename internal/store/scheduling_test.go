package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// A quest's scheduling fields — its Muster (the "when" date) and Recurrence (a
// "rite") — survive Save → Load → Save → Load with nothing dropped, including a
// weekly rite carrying explicit Weekdays. These are persisted; a round-trip
// regression would silently un-schedule a user's errands and recurring duties.
func TestRoundTripPreservesScheduling(t *testing.T) {
	muster := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	orig := &Store{
		Quests: []model.Quest{
			{
				ID:       "e1",
				Title:    "Review PR #123",
				BannerID: "errands",
				Muster:   &muster, // a one-off errand mustered for a day
				Status:   model.StatusOpen,
			},
			{
				ID:         "r1",
				Title:      "Check emails",
				BannerID:   "errands",
				Muster:     &muster,
				Recurrence: &model.Recurrence{Every: model.RecurDaily, Interval: 1}, // daily rite
			},
			{
				ID:       "r2",
				Title:    "Update priorities doc",
				BannerID: "errands",
				Muster:   &muster,
				Recurrence: &model.Recurrence{ // weekly rite pinned to specific weekdays
					Every:    model.RecurWeekly,
					Interval: 2,
					Weekdays: []time.Weekday{time.Monday, time.Thursday},
				},
			},
			{ID: "q1", Title: "Plain quest, no schedule"}, // nil Muster/Recurrence must stay absent
		},
	}
	path := filepath.Join(t.TempDir(), "data.json")

	if err := Save(path, orig); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, loaded); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	q := map[string]model.Quest{}
	for _, x := range s.Quests {
		q[x.ID] = x
	}

	if got := q["e1"].Muster; got == nil || !got.Equal(muster) {
		t.Fatalf("errand muster lost: %v, want %v", got, muster)
	}
	if q["e1"].Recurrence != nil {
		t.Errorf("a one-off errand gained a recurrence: %+v", q["e1"].Recurrence)
	}

	dr := q["r1"].Recurrence
	if dr == nil || dr.Every != model.RecurDaily || dr.Interval != 1 {
		t.Fatalf("daily rite lost/changed: %+v", dr)
	}

	wr := q["r2"].Recurrence
	if wr == nil || wr.Every != model.RecurWeekly || wr.Interval != 2 {
		t.Fatalf("weekly rite lost/changed: %+v", wr)
	}
	if len(wr.Weekdays) != 2 || wr.Weekdays[0] != time.Monday || wr.Weekdays[1] != time.Thursday {
		t.Errorf("weekly rite weekdays lost: %+v", wr.Weekdays)
	}

	if q["q1"].Muster != nil || q["q1"].Recurrence != nil {
		t.Errorf("an unscheduled quest gained scheduling: muster=%v recurrence=%+v", q["q1"].Muster, q["q1"].Recurrence)
	}
}
