package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Clicking "← back (esc)" in a quest detail page closes it (the hit-test reads
// focusLeftMargin, which viewQuestDetail must set — it previously didn't).
func TestQuestDetailBackClickCloses(t *testing.T) {
	ui.Init(true)
	st := &store.Store{Quests: []model.Quest{{ID: "q1", Title: "Q"}}}
	m := &Model{store: st, modal: questDetailModal(&st.Quests[0]),
		sectionScroll: map[string]int{}, sectionMaxScroll: map[string]int{}}
	m.modal.BodyEditor = m.newBodyEditor("")
	m.width, m.height = 100, 40
	m.detailWidthRatio = 0.42
	_ = m.renderContent() // populate focusLeftMargin / focusBackWidth / focusHeaderRow

	// Click the middle of the back label on the header row.
	m.handleFocusClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.focusLeftMargin + 1, Y: m.focusHeaderRow})
	if m.modal != nil {
		t.Fatalf("clicking back should close the quest detail, modal=%v", m.modal)
	}
}

// A click anywhere dismisses an open help modal.
func TestHelpModalClosesOnClick(t *testing.T) {
	m := &Model{modal: helpModal()}
	if _, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: 3}); m.modal != nil {
		t.Fatal("a click should dismiss the help modal")
	}
}
