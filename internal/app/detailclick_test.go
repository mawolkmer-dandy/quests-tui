package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Clicking an objective's checkbox inside a quest's detail marks it done (with
// the completion burst), same as the Wilds.
func TestDetailCheckboxClickTogglesDone(t *testing.T) {
	ui.Init(true)
	q := model.Quest{ID: "q1", Title: "Q", Body: []model.BodyLine{{ID: "o1", Text: "- task"}}}
	st := &store.Store{Quests: []model.Quest{q}}
	m := &Model{store: st, modal: questDetailModal(&st.Quests[0])}
	m.modal.BodyEditor = m.newBodyEditor("- task")
	// Geometry the click handler reads.
	m.focusBodyX = 10
	m.focusBodyBaseRow = 5
	m.focusRowLine = []int{0}
	m.focusRowOffset = []int{0}

	// Click the checkbox column (bodyObjCol=2 → focusBodyX+2 .. focusBodyX+4).
	m.handleFocusPointer(tea.Mouse{X: m.focusBodyX + bodyObjCol, Y: m.focusBodyBaseRow}, true)

	if !st.Quests[0].Body[0].Done {
		t.Fatal("clicking the checkbox should mark the objective done")
	}
	if len(m.overlayParticles) == 0 {
		t.Fatal("marking done should fire the completion burst")
	}
}

// Double-clicking a quest row opens its detail (mouse equivalent of Tab); a
// single click just selects.
func TestDoubleClickQuestOpensDetail(t *testing.T) {
	ui.Init(true)
	st := &store.Store{Projects: []model.Project{{ID: "p1", Name: "C"}}, Quests: []model.Quest{
		{ID: "q1", Title: "Q", ProjectID: "p1", Status: model.StatusOpen},
	}}
	m := &Model{store: st, wilds: false, collapsedProjects: map[string]bool{}, collapsedSections: map[string]bool{}}
	m.leftMargin = 0
	rows := m.visibleRows()
	var qi int = -1
	for i, r := range rows {
		if r.Kind == ui.RowQuest {
			qi = i
			break
		}
	}
	if qi < 0 {
		t.Fatal("no quest row")
	}
	click := func() { m.clickRowAt(rows, qi, tea.Mouse{X: 20, Y: qi}, 0) }
	click() // first: select only
	if m.modal != nil {
		t.Fatal("single click must not open the detail")
	}
	click() // second within window: open
	if m.modal == nil || m.modal.Kind != ModalQuestDetail {
		t.Fatalf("double click should open the quest detail, modal=%v", m.modal)
	}
}
