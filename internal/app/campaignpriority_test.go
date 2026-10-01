package app

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Ctrl+P cycles a campaign's priority both in the pane (via the row cursor) and
// in the hall (via the hall selection).
func TestCampaignPriorityCycles(t *testing.T) {
	ui.Init(true)
	newM := func() *Model {
		return &Model{
			store: &store.Store{
				Banners:  []model.Banner{{ID: "b1", Name: "Platform"}},
				Projects: []model.Project{{ID: "p1", Name: "Migrate", BannerID: "b1"}},
			},
			path:              filepath.Join(t.TempDir(), "data.json"),
			collapsedProjects: map[string]bool{},
			width:             120,
			height:            40,
		}
	}
	ctrlP := tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}

	// In the pane, on the campaign row.
	m := newM()
	m.hallFocus = false
	m.cursor = cursorTarget{kind: ui.RowProject, projectID: "p1"}
	m.handleKey(ctrlP)
	if got := m.findProject("p1").Priority; got != model.PriorityMedium {
		t.Fatalf("pane Ctrl+P should cycle campaign priority (none→medium), got %q", got)
	}

	// In the hall, on the campaign selection.
	m2 := newM()
	m2.hallFocus = true
	m2.hallCursor = cursorTarget{kind: ui.RowProject, projectID: "p1"}
	m2.handleKey(ctrlP)
	if got := m2.findProject("p1").Priority; got != model.PriorityMedium {
		t.Fatalf("hall Ctrl+P should cycle campaign priority, got %q", got)
	}
}

// A quest's and a campaign's priority arrow both render in the Tavern (Bare)
// rows — they no longer drop the priority slot.
func TestPriorityVisibleInTavern(t *testing.T) {
	ui.Init(true)
	q := model.Quest{ID: "q", Title: "Fix", ProjectID: "p1", Priority: model.PriorityHigh}
	qLine, _ := ui.RenderRow(ui.Row{Kind: ui.RowQuest, QuestID: "q", Bare: true}, &store.Store{Quests: []model.Quest{q}}, "", false, false, 80, "")
	if qLine == "" || !containsGlyph(qLine, ui.GlyphImportant) {
		t.Errorf("a high-priority quest should show its arrow in the Tavern (Bare) row: %q", qLine)
	}

	p := model.Project{ID: "p1", Name: "Migrate", Priority: model.PriorityHigh}
	pLine, _ := ui.RenderRow(ui.Row{Kind: ui.RowProject, ProjectID: "p1", Bare: true}, &store.Store{Projects: []model.Project{p}}, "", false, false, 80, "")
	if pLine == "" || !containsGlyph(pLine, ui.GlyphImportant) {
		t.Errorf("a high-priority campaign should show its arrow in the Tavern row: %q", pLine)
	}
}

// A banner/Errands pane sorts its loose quests by priority — a low-priority
// quest sinks below a no-priority one, even when it's earlier in store order.
func TestBannerPaneSortsLooseByPriority(t *testing.T) {
	ui.Init(true)
	ui.LowPriorityToBottom = true // set from config in prod; tests must set it themselves
	defer func() { ui.LowPriorityToBottom = false }()

	m := &Model{
		store: &store.Store{
			Quests: []model.Quest{
				{ID: "low", Title: "Low", BannerID: errandsBanner, Priority: model.PriorityLow}, // first in store
				{ID: "none", Title: "None", BannerID: errandsBanner},
			},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}

	noneIdx, lowIdx := -1, -1
	i := 0
	for _, r := range m.bannerPaneRows(errandsBanner) {
		if r.Kind != ui.RowQuest {
			continue
		}
		switch r.QuestID {
		case "none":
			noneIdx = i
		case "low":
			lowIdx = i
		}
		i++
	}
	if noneIdx < 0 || lowIdx < 0 || noneIdx > lowIdx {
		t.Fatalf("no-priority quest should sort before low-priority (none=%d low=%d)", noneIdx, lowIdx)
	}
}

func containsGlyph(s, glyph string) bool {
	for i := 0; i+len(glyph) <= len(s); i++ {
		if s[i:i+len(glyph)] == glyph {
			return true
		}
	}
	return false
}
