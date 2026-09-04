package app

import (
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// ventureModel is a Camp-ready model: one campaign with one taken-up (active)
// quest, launched into Camp (wilds), animations off so transitions are instant.
func ventureModel(t *testing.T, q model.Quest) *Model {
	ui.Init(true)
	m := &Model{
		store: &store.Store{
			Projects: []model.Project{{ID: "p1", Name: "Migrate"}},
			Quests:   []model.Quest{q},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		wilds:             true,
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{},
		sectionScroll:     map[string]int{},
		sectionMaxScroll:  map[string]int{},
	}
	m.width, m.height = 100, 30
	m.lastSnapshot = m.store.Snapshot()
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	}
	return m
}

func activeQuest() model.Quest {
	return model.Quest{ID: "q1", Title: "Fix the parser", ProjectID: "p1", Status: model.StatusActive}
}

// Venturing from Camp narrows the view to the single quest (+ its pending
// objectives) and starts a session; the header shows the timer.
func TestVentureNarrowsToOneQuest(t *testing.T) {
	q := activeQuest()
	q.Body = []model.BodyLine{{ID: "o1", Text: "- parse the header"}}
	m := ventureModel(t, q)

	m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: "p1", QuestID: "q1"})
	m.venture("q1")

	if !m.venturing() {
		t.Fatal("venture should put the model into the Wilds")
	}
	rows := m.visibleRows()
	if len(rows) != 2 || rows[0].Kind != ui.RowQuest || rows[0].QuestID != "q1" || rows[1].Kind != ui.RowWildsObjective {
		t.Fatalf("Wilds should show just the quest + its pending objective, got %+v", rows)
	}
}

// Ctrl+D on the ventured quest finishes it: quest done, back at Camp, one
// completed session logged.
func TestCompleteVentureReturnsToCampDone(t *testing.T) {
	m := ventureModel(t, activeQuest())
	m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: "p1", QuestID: "q1"})
	m.venture("q1")

	m.completeVenture()

	if m.venturing() {
		t.Fatal("completing should return to Camp")
	}
	if q := m.findQuest("q1"); q == nil || q.Status != model.StatusDone {
		t.Fatal("completing should mark the quest done")
	}
	if n := len(m.store.WildsSessions); n != 1 {
		t.Fatalf("completing should log exactly one session, got %d", n)
	}
	if !m.store.WildsSessions[0].Completed {
		t.Fatal("the logged session should be marked completed")
	}
}

// Esc "makes camp": back to Camp, session logged but NOT completed, quest left
// as-is (no penalty for bailing).
func TestMakeCampBailLogsIncompleteSession(t *testing.T) {
	m := ventureModel(t, activeQuest())
	m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: "p1", QuestID: "q1"})
	m.venture("q1")

	m.makeCamp(false)

	if m.venturing() {
		t.Fatal("make camp should leave the Wilds")
	}
	if q := m.findQuest("q1"); q == nil || q.Status != model.StatusActive {
		t.Fatal("bailing must not change the quest's status")
	}
	if n := len(m.store.WildsSessions); n != 1 || m.store.WildsSessions[0].Completed {
		t.Fatalf("bailing should log one non-completed session, got %+v", m.store.WildsSessions)
	}
}

// Checking the last pending objective out in the Wilds finishes the quest —
// the completion moment of the ritual.
func TestLastObjectiveCompletesVenture(t *testing.T) {
	q := activeQuest()
	q.Body = []model.BodyLine{{ID: "o1", Text: "- the only step"}}
	m := ventureModel(t, q)

	m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: "p1", QuestID: "q1"})
	m.venture("q1")
	// Land on the objective and check it off.
	m.setCursor(ui.Row{Kind: ui.RowWildsObjective, ProjectID: "p1", QuestID: "q1", BodyLineID: "o1"})
	m.markWildsObjectiveDone()

	if m.venturing() {
		t.Fatal("clearing the last objective should finish the venture")
	}
	if q := m.findQuest("q1"); q == nil || q.Status != model.StatusDone {
		t.Fatal("finishing via the last objective should mark the quest done")
	}
	if n := len(m.store.WildsSessions); n != 1 || !m.store.WildsSessions[0].Completed {
		t.Fatalf("should log one completed session, got %+v", m.store.WildsSessions)
	}
}

// Every venture round-trip logs exactly one session (the durable focus record).
func TestVentureLogsOneSession(t *testing.T) {
	m := ventureModel(t, activeQuest())
	m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: "p1", QuestID: "q1"})
	m.venture("q1")
	m.makeCamp(false)
	if len(m.store.WildsSessions) != 1 {
		t.Fatalf("expected one logged session, got %d", len(m.store.WildsSessions))
	}
}
