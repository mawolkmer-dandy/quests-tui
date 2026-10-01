package app

import (
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Tab on a sigil row in the Trails/Lookouts/Runes rooms opens the quest that the
// sigil belongs to — so you can see what references it. Covers every sigil kind
// (the group header and the individual lines).
func TestTabOnSigilOpensOwningQuest(t *testing.T) {
	ui.Init(true)
	kinds := []ui.RowKind{
		ui.RowRune, ui.RowRuneQuest,
		ui.RowLookout, ui.RowLookoutQuest,
		ui.RowTrack, ui.RowTrail, ui.RowTrailQuest,
	}
	for _, k := range kinds {
		m := &Model{
			store: &store.Store{
				Quests: []model.Quest{{ID: "q1", Title: "Adoption tracking"}},
			},
			path:              filepath.Join(t.TempDir(), "data.json"),
			collapsedProjects: map[string]bool{},
		}
		m.cursor = cursorTarget{kind: k, questID: "q1"}

		m.handleReveal() // Tab

		if m.modal == nil || m.modal.Kind != ModalQuestDetail || m.modal.QuestID != "q1" {
			t.Fatalf("Tab on %v should open quest q1's detail; modal=%+v", k, m.modal)
		}
	}
}
