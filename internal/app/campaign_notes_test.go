package app

import (
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// A text selection that spans two campaign-note lines registers as multiline —
// so it highlights and copies across lines, the same as a quest body. This
// broke once (multilineSelActive only recognized focus modals, not pane notes).
func TestCampaignNoteSelectionSpansLines(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store: &store.Store{Projects: []model.Project{{ID: "c1", Name: "C", Body: []model.BodyLine{
			{ID: "n1", Text: "line one"},
			{ID: "n2", Text: "line two"},
		}}}},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}
	m.hallCursor = cursorTarget{kind: ui.RowProject, projectID: "c1"}
	m.enterCampaignNotes(ui.Row{Kind: ui.RowBodyLine, ProjectID: "c1", BodyLineID: "n1"})
	m.cursor = cursorTarget{kind: ui.RowBodyLine, projectID: "c1", bodyLineID: "n1"}

	// Caret extended down to line 1 (seed the editor first — newBodyEditor
	// clears any selection), then anchor the selection back on line 0.
	m.bodyCursor = 1
	m.bodyEditor = m.newBodyEditor("line two")
	m.bodyEditor.SetCursor(4)
	m.selAnchor, m.selAnchorLine = 0, 0

	if !m.multilineSelActive() {
		t.Fatal("a selection spanning note lines should be multiline-active")
	}
	// Line 0 is fully within the selection; line 1 up to the caret.
	if lo, hi, ok := m.bodyLineSelRange(0, len("line one")); !ok || lo != 0 || hi != len("line one") {
		t.Fatalf("line 0 range = (%d,%d,%v), want full line", lo, hi, ok)
	}
	if lo, hi, ok := m.bodyLineSelRange(1, len("line two")); !ok || lo != 0 || hi != 4 {
		t.Fatalf("line 1 range = (%d,%d,%v), want [0,4)", lo, hi, ok)
	}
}
