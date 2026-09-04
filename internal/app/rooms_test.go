package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func roomsModel() *Model {
	ui.Init(true)
	m := flickerModel(28)
	m.width = 110
	m.store = &store.Store{
		Projects: []model.Project{{ID: "p1", Name: "Homestead"}},
		Quests: []model.Quest{
			{ID: "q1", Title: "Clear the cellar", ProjectID: "p1", Status: model.StatusActive},
			{ID: "s1", Title: "A stranger has a job"}, // Questboard (no project)
		},
	}
	if rows := m.visibleRows(); len(rows) > 0 {
		if r, ok := nearestSelectableRow(rows, 0); ok {
			m.setCursor(r)
		}
	}
	return m
}

func rowsHaveQuest(rows []ui.Row, id string) bool {
	for _, r := range rows {
		if r.Kind == ui.RowQuest && r.QuestID == id {
			return true
		}
	}
	return false
}

// The hall selection drives the pane: pick the Questboard room and the pane
// shows its notice; pick the campaign and the pane shows the campaign's quest.
func TestHallSelectionDrivesPane(t *testing.T) {
	m := roomsModel()
	_ = frame(m) // establish the hall + pane geometry

	m.selectHallTarget(cursorTarget{kind: ui.RowSection, section: "inbox"})
	if !rowsHaveQuest(m.paneRows(), "s1") {
		t.Fatal("the Questboard pane should show the questboard quest s1")
	}
	if rowsHaveQuest(m.paneRows(), "q1") {
		t.Fatal("the Questboard pane should not show the campaign quest q1")
	}

	m.selectHallTarget(cursorTarget{kind: ui.RowProject, projectID: "p1"})
	if !rowsHaveQuest(m.paneRows(), "q1") {
		t.Fatal("the campaign pane should show its quest q1")
	}
}

// Tab dives from the hall into the pane (landing on the first content row); Esc
// returns focus to the hall.
func TestHallDiveAndReturn(t *testing.T) {
	m := roomsModel()
	_ = frame(m)
	m.selectHallTarget(cursorTarget{kind: ui.RowProject, projectID: "p1"})
	if !m.hallFocus {
		t.Fatal("selecting an entry keeps focus in the hall")
	}
	m.diveIntoPane()
	if m.hallFocus {
		t.Fatal("diving should move focus into the pane")
	}
	// The campaign header leads the pane, so a dive lands on it (its name is
	// editable here); pressing Down from there reaches the quests.
	if m.cursor.kind != ui.RowProject || m.cursor.projectID != "p1" {
		t.Fatalf("dive should land on the campaign header, got %+v", m.cursor)
	}
	m.returnToHall()
	if !m.hallFocus {
		t.Fatal("Esc should return focus to the hall")
	}
}

// A hall taller than the viewport scrolls to follow the cursor as it walks
// down past the fold.
func TestHallBodyScrolls(t *testing.T) {
	m := roomsModel()
	m.height = 16 // a short screen so the hall overflows
	for i := 0; i < 20; i++ {
		m.store.Banners = append(m.store.Banners, model.Banner{ID: "b" + string(rune('a'+i)), Name: "Area " + string(rune('a'+i))})
	}
	if r, ok := nearestSelectableRow(m.hallRows(), 0); ok {
		m.hallCursor = targetFromRow(r)
	}
	m.hallFocus = true
	_ = frame(m)
	for i := 0; i < 30; i++ { // walk to the bottom of the hall
		m.moveHallCursor(1)
		_ = frame(m)
	}
	if m.hallScroll <= 0 {
		t.Fatalf("the hall should have scrolled walking to the bottom, got hallScroll=%d", m.hallScroll)
	}
}

// The Ctrl+P fuzzy jump navigates the hall straight to a chosen campaign.
func TestJumpNavigatesHall(t *testing.T) {
	m := roomsModel()
	_ = frame(m)
	m.modal = jumpModal(m.store)
	idx := -1
	for i, it := range m.modal.PickerItems {
		if it.ID == "project:p1" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("the jump list should include the campaign")
	}
	m.modal.PickerIndex = idx
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.hallCursor.kind != ui.RowProject || m.hallCursor.projectID != "p1" {
		t.Fatalf("jump should select the campaign in the hall, got %+v", m.hallCursor)
	}
	if m.modal != nil {
		t.Fatal("confirming the jump should close the picker")
	}
}
