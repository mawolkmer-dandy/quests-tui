package app

import (
	"strings"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// A focused quest still shows its inline connection emblems, and those emblems
// (plus objective progress) sit at the SAME column whether the edit caret is
// mid-name or at the end — no 1-column jump.
func TestQuestEmblemsStableAcrossCaret(t *testing.T) {
	ui.Init(true)
	st := &store.Store{Projects: []model.Project{{ID: "p1", Name: "C"}}, Quests: []model.Quest{
		{ID: "q1", Title: "Hello world", ProjectID: "p1", Status: model.StatusOpen, JiraCodes: []string{"EPDCHAIR-1"}},
	}}
	m := &Model{store: st, integrationsEnabled: true, collapsedProjects: map[string]bool{}, collapsedSections: map[string]bool{},
		jiraStatus: map[string]JiraStatus{}, prStatus: map[string]PRStatus{}, runeStatus: map[string]RuneStatus{}}
	rows := m.visibleRows()
	var qi int
	for i, r := range rows {
		if r.Kind == ui.RowQuest {
			qi = i
			m.setCursor(r)
			break
		}
	}
	emblemCol := func() int {
		return strings.Index(stripANSI(m.renderOutlineRowLine(rows, qi, qi, -1, -1, 120, 0)), ui.GlyphConnScroll)
	}
	m.editor.CursorEnd()
	end := emblemCol()
	if end < 0 {
		t.Fatal("a focused quest must still show its connection emblems")
	}
	m.editor.SetCursor(3)
	if mid := emblemCol(); mid != end {
		t.Fatalf("emblem jumped when caret moved to end: mid=%d end=%d", mid, end)
	}
}
