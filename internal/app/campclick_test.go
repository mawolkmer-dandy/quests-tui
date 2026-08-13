package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Clicking a quest in a campaign detail page selects it (enters the quest list);
// double-click opens its detail. Previously all clicks there were dropped.
func TestCampaignPageClickQuest(t *testing.T) {
	ui.Init(true)
	st := &store.Store{Projects: []model.Project{{ID: "p1", Name: "Camp"}}, Quests: []model.Quest{
		{ID: "q1", Title: "First", ProjectID: "p1", Status: model.StatusOpen},
		{ID: "q2", Title: "Second", ProjectID: "p1", Status: model.StatusOpen},
	}}
	m := &Model{store: st, collapsedProjects: map[string]bool{}, collapsedSections: map[string]bool{},
		sectionScroll: map[string]int{}, sectionMaxScroll: map[string]int{}}
	m.width, m.height = 100, 40
	m.modal = campaignDetailModal(&st.Projects[0])
	_ = m.renderContent() // populate focusContentTop / focusQuestListStart / focusLeftMargin

	qrows := campaignQuestRows(m.store, "p1")
	if len(qrows) < 1 {
		t.Fatal("no campaign quest rows")
	}
	y := m.focusContentTop + m.focusQuestListStart // first quest row
	click := func() { m.handleFocusPointer(tea.Mouse{X: m.focusLeftMargin + 20, Y: y}, true) }

	click()
	if !m.modal.InQuestList {
		t.Fatal("clicking a quest should enter the quest list")
	}
	if !m.cursor.matches(qrows[0]) {
		t.Fatalf("cursor should be on the clicked quest, got %+v", m.cursor)
	}
	click() // double-click within window → open
	if m.modal == nil || m.modal.Kind != ModalQuestDetail {
		t.Fatalf("double-click should open the quest detail, got %v", m.modal)
	}
}
