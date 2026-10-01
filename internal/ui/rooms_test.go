package ui

import (
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
)

// The Runes and Lookouts rooms match the Trails room: quest headers are not
// collapsible, so every item shows regardless of any stale collapse state.
func TestRunesLookoutsNotCollapsible(t *testing.T) {
	Init(true)
	s := &store.Store{Quests: []model.Quest{{
		ID: "q", Title: "Q",
		Runes:    []string{"flag_a", "flag_b"},
		Lookouts: []model.Lookout{{URL: "https://x"}},
	}}}
	collapsed := map[string]bool{"q": true} // would have folded the group before

	runeItems := 0
	for _, r := range SectionContent(s, "runes", collapsed) {
		if r.Kind == RowRune {
			runeItems++
		}
		if r.Kind == RowRuneQuest && r.Collapsed {
			t.Error("a rune quest header must never be collapsed")
		}
	}
	if runeItems != 2 {
		t.Errorf("both runes should show regardless of collapse, got %d", runeItems)
	}

	lookItems := 0
	for _, r := range SectionContent(s, "lookouts", collapsed) {
		if r.Kind == RowLookout {
			lookItems++
		}
		if r.Kind == RowLookoutQuest && r.Collapsed {
			t.Error("a lookout quest header must never be collapsed")
		}
	}
	if lookItems != 1 {
		t.Errorf("the lookout should show regardless of collapse, got %d", lookItems)
	}
}
