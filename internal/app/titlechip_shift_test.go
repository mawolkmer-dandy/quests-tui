package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The chip after the quest-detail title must sit at the SAME column whether the
// rename caret is mid-title or at the end (shared reserve-caret helper).
func TestDetailTitleChipStableAcrossCaret(t *testing.T) {
	ui.Init(true)
	st := &store.Store{Quests: []model.Quest{{ID: "q1", Title: "Hello world", Status: model.StatusOpen}}}
	m := &Model{store: st, modal: questDetailModal(&st.Quests[0]),
		sectionScroll: map[string]int{}, sectionMaxScroll: map[string]int{}}
	m.modal.BodyEditor = m.newBodyEditor("")
	m.width, m.height = 100, 40
	m.detailWidthRatio = 0.42

	// The chip's DISPLAY column on the title line (not a byte offset — the edit
	// cursor mark uses a multi-byte rune, so byte offsets would differ spuriously).
	chipCol := func() int {
		for _, line := range strings.Split(stripANSI(m.viewQuestDetail()), "\n") {
			if i := strings.Index(line, "·"); i >= 0 {
				return lipgloss.Width(line[:i])
			}
		}
		return -1
	}

	// Not editing.
	plain := chipCol()
	if plain < 0 {
		t.Fatal("chip not found")
	}
	// Selecting the title to edit must NOT move the chip.
	m.beginTitleEdit()
	m.titleEditor.CursorEnd()
	if end := chipCol(); end != plain {
		t.Fatalf("chip shifted on select: plain col %d vs editing(end) col %d", plain, end)
	}
	// Moving the caret within the title must NOT move the chip either.
	m.titleEditor.SetCursor(3)
	if mid := chipCol(); mid != plain {
		t.Fatalf("chip shifted with caret: plain col %d vs editing(mid) col %d", plain, mid)
	}
}
