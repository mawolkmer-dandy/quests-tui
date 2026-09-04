package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func wildsObjModel(body ...model.BodyLine) *Model {
	ui.Init(true)
	st := &store.Store{
		Projects: []model.Project{{ID: "p1", Name: "C"}},
		Quests: []model.Quest{{
			ID: "q1", Title: "Q", ProjectID: "p1", Status: model.StatusActive, Body: body,
		}},
	}
	m := &Model{store: st, wilds: true, collapsedProjects: map[string]bool{}, collapsedSections: map[string]bool{}}
	m.leftMargin = 0
	return m
}

func wildsObjRowIndex(t *testing.T, m *Model, rows []ui.Row) int {
	t.Helper()
	for i, r := range rows {
		if r.Kind == ui.RowWildsObjective {
			return i
		}
	}
	t.Fatal("no wilds objective row")
	return -1
}

// Clicking an objective's TEXT (not its checkbox) must not mark it done — it
// enters inline editing, exactly like clicking a quest title.
func TestWildsObjectiveTextClickEditsNotDone(t *testing.T) {
	m := wildsObjModel(model.BodyLine{ID: "o1", Text: "- first"})
	rows := m.visibleRows()
	idx := wildsObjRowIndex(t, m, rows)

	textX := objectiveTextOffset(0) + 2 // a column inside the objective text
	m.overlayParticles = nil
	m.clickRowAt(rows, idx, tea.Mouse{X: textX, Y: idx}, m.leftMargin)

	if m.store.Quests[0].Body[0].Done {
		t.Fatal("clicking the text must NOT mark the objective done")
	}
	if len(m.overlayParticles) != 0 {
		t.Fatal("clicking the text must not fire a completion burst")
	}
	if m.editor == nil || m.cursor.kind != ui.RowWildsObjective {
		t.Fatalf("clicking the text should start editing the objective (editor=%v kind=%v)", m.editor != nil, m.cursor.kind)
	}
}

// The checkbox column still marks done (regression guard for the split zone).
func TestWildsObjectiveCheckboxClickMarksDone(t *testing.T) {
	m := wildsObjModel(model.BodyLine{ID: "o1", Text: "- first"})
	rows := m.visibleRows()
	idx := wildsObjRowIndex(t, m, rows)

	m.clickRowAt(rows, idx, tea.Mouse{X: wildsObjCol, Y: idx}, m.leftMargin)

	if !m.store.Quests[0].Body[0].Done {
		t.Fatal("clicking the checkbox should mark the objective done")
	}
}

// Double-clicking an objective opens its PARENT quest's detail (single click
// only edits).
func TestWildsObjectiveDoubleClickOpensQuest(t *testing.T) {
	m := wildsObjModel(model.BodyLine{ID: "o1", Text: "- first"})
	rows := m.visibleRows()
	idx := wildsObjRowIndex(t, m, rows)

	textX := objectiveTextOffset(0) + 2
	click := func() { m.clickRowAt(rows, idx, tea.Mouse{X: textX, Y: idx}, m.leftMargin) }

	click()
	if m.modal != nil {
		t.Fatal("single click must not open the quest")
	}
	click()
	if m.modal == nil || m.modal.Kind != ModalQuestDetail || m.modal.QuestID != "q1" {
		t.Fatalf("double click should open the parent quest detail, modal=%v", m.modal)
	}
}

// Editing an objective rewrites only its display text, preserving the "- "
// marker and the line's ID / Done / Indent — and never disturbs sibling lines.
func TestWildsObjectiveEditPreservesFields(t *testing.T) {
	m := wildsObjModel(
		model.BodyLine{ID: "h1", Text: "# Heading"},
		model.BodyLine{ID: "o1", Text: "- first", Done: false, Indent: 2},
		model.BodyLine{ID: "t1", Text: "some note"},
	)
	row := ui.Row{Kind: ui.RowWildsObjective, ProjectID: "p1", QuestID: "q1", BodyLineID: "o1"}

	m.setCursor(row)
	if m.editor == nil {
		t.Fatal("selecting an objective must bind an editor")
	}
	if got := m.editor.Value(); got != "first" {
		t.Fatalf("editor seeded with %q, want the display text %q", got, "first")
	}
	m.editor.SetValue("renamed task")
	m.commitEdit()

	body := m.store.Quests[0].Body
	obj := body[1]
	if obj.Text != "- renamed task" {
		t.Fatalf("Text=%q, want %q (marker preserved)", obj.Text, "- renamed task")
	}
	if obj.ID != "o1" || obj.Done || obj.Indent != 2 {
		t.Fatalf("edit disturbed other fields: %+v", obj)
	}
	if body[0].Text != "# Heading" || body[2].Text != "some note" {
		t.Fatal("edit must not touch sibling body lines")
	}
}
