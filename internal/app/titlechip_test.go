package app

import (
	"strings"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The type/status chip after a quest title stays visible while the title is
// being renamed.
func TestTitleChipShownWhileRenaming(t *testing.T) {
	ui.Init(true)
	st := &store.Store{Quests: []model.Quest{{ID: "q1", Title: "Q", Status: model.StatusOpen}}}
	m := &Model{store: st, modal: questDetailModal(&st.Quests[0]),
		sectionScroll: map[string]int{}, sectionMaxScroll: map[string]int{}}
	m.modal.BodyEditor = m.newBodyEditor("")
	m.width, m.height = 100, 40
	m.detailWidthRatio = 0.42

	chipMark := m.questStatusLabel(&st.Quests[0]) // part of the chip
	m.beginTitleEdit()
	out := stripANSI(m.viewQuestDetail())
	if !strings.Contains(out, chipMark) {
		t.Fatalf("chip (%q) hidden while renaming the title:\n%s", chipMark, out)
	}
}
