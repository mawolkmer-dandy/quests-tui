package app

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Ctrl+P always cycles a quest's priority (never opens a finder), and Ctrl+F
// opens the unified Search modal — the two no longer collide.
func TestCtrlPPriorityAndCtrlFSearch(t *testing.T) {
	ui.Init(true)
	newM := func() *Model {
		return &Model{
			store: &store.Store{
				Banners:  []model.Banner{{ID: "b1", Name: "Platform"}},
				Projects: []model.Project{{ID: "p1", Name: "Migrate", BannerID: "b1"}},
				Quests:   []model.Quest{{ID: "q", Title: "T", ProjectID: "p1"}},
			},
			path:              filepath.Join(t.TempDir(), "data.json"),
			collapsedProjects: map[string]bool{},
			width:             120,
			height:            40,
		}
	}

	// Ctrl+P on a quest → cycles priority, no modal.
	m := newM()
	m.hallFocus = false
	m.cursor = cursorTarget{kind: ui.RowQuest, questID: "q"}
	m.handleKey(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if m.modal != nil {
		t.Fatal("Ctrl+P must not open a modal")
	}
	if got := m.findQuest("q").Priority; got != model.PriorityMedium {
		t.Fatalf("Ctrl+P on a quest should cycle priority (none→medium), got %q", got)
	}

	// Ctrl+F → opens the Search modal.
	m2 := newM()
	m2.hallFocus = true
	m2.hallCursor = cursorTarget{kind: ui.RowBanner, bannerID: "b1"}
	m2.handleKey(tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if m2.modal == nil || m2.modal.Kind != ModalProjectPicker || !m2.modal.Jump {
		t.Fatalf("Ctrl+F should open the Search modal, got %+v", m2.modal)
	}
}
