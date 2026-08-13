package app

import (
	"strings"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// A quest row shows its inline connection emblems on EVERY surface that renders
// it — outline, Tavern rail box, Tavern campaigns column, and the campaign /
// section focus pages. Guards against the surfaces drifting apart again (see
// docs/ui-consistency.md).
func TestQuestEmblemsOnEverySurface(t *testing.T) {
	ui.Init(true)
	st := &store.Store{
		Projects: []model.Project{{ID: "p1", Name: "Camp"}},
		Quests: []model.Quest{
			{ID: "q1", Title: "A quest", ProjectID: "p1", Status: model.StatusOpen, JiraCodes: []string{"EPDCHAIR-1"}},
		},
	}
	m := &Model{store: st, integrationsEnabled: true, collapsedProjects: map[string]bool{}, collapsedSections: map[string]bool{},
		jiraStatus: map[string]JiraStatus{}, prStatus: map[string]PRStatus{}, runeStatus: map[string]RuneStatus{}}
	qrow := ui.Row{Kind: ui.RowQuest, QuestID: "q1", ProjectID: "p1"}
	hasEmblem := func(name, line string) {
		if !strings.Contains(stripANSI(line), ui.GlyphConnScroll) {
			t.Fatalf("%s: quest row missing its Jira emblem: %q", name, stripANSI(line))
		}
	}

	// Outline (single-column).
	orows := m.visibleRows()
	for i, r := range orows {
		if r.Kind == ui.RowQuest {
			hasEmblem("outline", m.renderOutlineRowLine(orows, i, -1, -1, -1, 120, 0))
		}
	}
	// Tavern rail box item.
	hasEmblem("rail box", m.renderBoxItemLine([]ui.Row{qrow}, 0, -1, 120))
	// Campaign / section focus page.
	hasEmblem("focus page", m.renderFocusListRow(qrow, false))
}
