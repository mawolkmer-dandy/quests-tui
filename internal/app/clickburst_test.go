package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Clicking an objective's checkbox to mark it done must fire the completion
// burst at the CLICKED row, not at the cursor's previous on-screen position.
func TestClickObjectiveBurstAtClickedRow(t *testing.T) {
	ui.Init(true)
	st := &store.Store{Projects: []model.Project{{ID: "p1", Name: "C"}}, Quests: []model.Quest{
		{ID: "q1", Title: "Q", ProjectID: "p1", Status: model.StatusActive, Body: []model.BodyLine{
			{ID: "o1", Text: "- first"},
			{ID: "o2", Text: "- second"},
		}},
	}}
	m := &Model{store: st, wilds: true, collapsedProjects: map[string]bool{}, collapsedSections: map[string]bool{}}
	m.leftMargin, m.rowsScreenTop = 2, 5
	// Stale cursor position from a previous render (far from the click).
	m.cursorScreenX, m.cursorScreenY = 2, 99

	rows := m.visibleRows()
	var objIdx int = -1
	for i, r := range rows {
		if r.Kind == ui.RowWildsObjective {
			objIdx = i
			break
		}
	}
	if objIdx < 0 {
		t.Fatal("no wilds objective row to click")
	}
	clickY := m.rowsScreenTop + objIdx // absolute screen row of that objective
	m.overlayParticles = nil
	m.clickRowAt(rows, objIdx, tea.Mouse{X: m.leftMargin + wildsObjCol, Y: clickY}, m.leftMargin)

	if len(m.overlayParticles) == 0 {
		t.Fatal("clicking an objective should spawn a completion burst")
	}
	for _, p := range m.overlayParticles {
		if int(p.y) != clickY {
			t.Fatalf("burst fired at y=%v, want the clicked row y=%d (not the stale cursor)", p.y, clickY)
		}
	}
}
