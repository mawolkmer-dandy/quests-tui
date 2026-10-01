package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Moving the mouse over the quest-detail body WITHOUT holding the button must
// not extend a text selection (v2 motion events carry no button state).
func TestBodyDragOnlyWhileButtonHeld(t *testing.T) {
	ui.Init(true)
	q := model.Quest{ID: "q1", Title: "Q", Body: []model.BodyLine{{ID: "l1", Text: "hello world"}}}
	st := &store.Store{Quests: []model.Quest{q}}
	m := &Model{store: st, modal: &Modal{Kind: ModalQuestDetail, QuestID: st.Quests[0].ID}}
	m.bodyOwnerKind, m.bodyOwnerID = ownerQuest, st.Quests[0].ID
	m.bodyEditor = m.newBodyEditor("hello world")
	m.focusBodyX = 0
	m.focusHeaderRow = 0
	m.focusBodyBaseRow = 5 // body rows well below the header
	m.focusRowLine = []int{0}
	m.focusRowOffset = []int{0}
	const bodyY = 5
	textCol := m.focusBodyX + 4

	// Press at column 0 → anchor set, button down.
	m.leftDown = true
	m.handleFocusPointer(tea.Mouse{X: textCol, Y: bodyY}, true)

	// Release (button up), then move the mouse — must NOT extend selection.
	m.leftDown = false
	m.handleFocusPointer(tea.Mouse{X: textCol + 5, Y: bodyY}, false)
	if _, _, has := m.selectionBounds(&m.bodyEditor); has {
		t.Fatal("selection extended on motion without the button held")
	}

	// Now with the button held, motion SHOULD extend the selection.
	m.leftDown = true
	m.handleFocusPointer(tea.Mouse{X: textCol + 5, Y: bodyY}, false)
	if _, _, has := m.selectionBounds(&m.bodyEditor); !has {
		t.Fatal("holding the button and moving should extend the selection")
	}
}
