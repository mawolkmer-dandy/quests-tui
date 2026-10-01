package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func campQuestIDs(m *Model) map[string]bool {
	out := map[string]bool{}
	for _, r := range m.wildsRows() {
		if r.Kind == ui.RowQuest {
			out[r.QuestID] = true
		}
	}
	return out
}

// Camp shows active quests plus "called" ones (muster today/overdue), and hides
// future-mustered, done, and unscheduled-inactive quests.
func TestCampSurfacesCalledQuests(t *testing.T) {
	ui.Init(true)
	today := model.DateOnly(time.Now())
	tomorrow := today.AddDate(0, 0, 1)
	yesterday := today.AddDate(0, 0, -1)

	m := &Model{
		store: &store.Store{
			Projects: []model.Project{{ID: "p1", Name: "Migrate"}},
			Quests: []model.Quest{
				{ID: "active", Title: "Taken up", ProjectID: "p1", Status: model.StatusActive},
				{ID: "idle", Title: "Backlog", ProjectID: "p1", Status: model.StatusOpen},
				{ID: "called", Title: "Review PR", BannerID: errandsBanner, Muster: &today},
				{ID: "overdue", Title: "Sign form", BannerID: errandsBanner, Muster: &yesterday},
				{ID: "future", Title: "Later errand", BannerID: errandsBanner, Muster: &tomorrow},
				{ID: "donecalled", Title: "Done today", BannerID: errandsBanner, Muster: &today, Status: model.StatusDone},
			},
		},
		path: filepath.Join(t.TempDir(), "data.json"),
	}

	got := campQuestIDs(m)
	want := map[string]bool{"active": true, "called": true, "overdue": true}
	for id := range want {
		if !got[id] {
			t.Errorf("Camp should surface %q", id)
		}
	}
	for _, id := range []string{"idle", "future", "donecalled"} {
		if got[id] {
			t.Errorf("Camp should NOT surface %q", id)
		}
	}
}

// Completing a rite recycles it: its muster rolls to the next occurrence, it
// returns to open (leaving Camp), and it never lands in the done state.
func TestToggleDoneRecyclesRite(t *testing.T) {
	ui.Init(true)
	today := model.DateOnly(time.Now())
	m := &Model{
		store: &store.Store{
			Quests: []model.Quest{{
				ID:         "r1",
				Title:      "Check emails",
				BannerID:   errandsBanner,
				Status:     model.StatusActive,
				Muster:     &today,
				Recurrence: &model.Recurrence{Every: model.RecurDaily, Interval: 1},
			}},
		},
		path: filepath.Join(t.TempDir(), "data.json"),
	}
	m.cursor = cursorTarget{kind: ui.RowQuest, questID: "r1"}
	m.toggleDone()

	q := m.findQuest("r1")
	if q.Status == model.StatusDone {
		t.Fatal("a rite must not finish — it recycles")
	}
	if q.Recurrence == nil {
		t.Fatal("recycling must keep the rite's recurrence")
	}
	if q.Muster == nil || !q.Muster.Equal(today.AddDate(0, 0, 1)) {
		t.Fatalf("a daily rite completed today should muster tomorrow, got %v", q.Muster)
	}
	if q.CompletedAt != nil {
		t.Error("a recycled rite should not carry a completion time")
	}
	// It has left Camp (mustered tomorrow, no longer active/called).
	if campQuestIDs(m)["r1"] {
		t.Error("a recycled rite should leave Camp until its next muster")
	}
}

// A one-off (non-rite) quest still finishes normally on complete.
func TestToggleDoneFinishesOneOff(t *testing.T) {
	ui.Init(true)
	today := model.DateOnly(time.Now())
	m := &Model{
		store: &store.Store{
			Quests: []model.Quest{{ID: "e1", Title: "Read doc", BannerID: errandsBanner, Status: model.StatusActive, Muster: &today}},
		},
		path: filepath.Join(t.TempDir(), "data.json"),
	}
	m.cursor = cursorTarget{kind: ui.RowQuest, questID: "e1"}
	m.toggleDone()

	if q := m.findQuest("e1"); q.Status != model.StatusDone || q.CompletedAt == nil {
		t.Fatalf("a one-off errand should finish normally, got status=%q completed=%v", q.Status, q.CompletedAt)
	}
}
