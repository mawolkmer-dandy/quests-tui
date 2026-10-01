package app

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// A shortened link in a campaign note is click-to-copy / double-click-to-open —
// the same gesture as a quest body link. Rendering the Tavern records the link's
// clickable span; a click there routes through clickLink (first copies, a fast
// second opens and disarms).
func TestCampaignNoteLinkClick(t *testing.T) {
	ui.Init(true)
	const full = "https://docs.google.com/document/d/1FmbgLVtTtByAbCdEfGh/edit?tab=t.0"
	short := "docs.google.com/document/d/1Fmbg…/edit"
	m := &Model{
		store: &store.Store{
			Banners: []model.Banner{{ID: "b1", Name: "Platform"}},
			Projects: []model.Project{{ID: "p1", Name: "Migrate", BannerID: "b1",
				Body:      []model.BodyLine{{ID: "n1", Text: "Spec: " + short}},
				BodyLinks: map[string]string{short: full}}},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{},
		sectionScroll:     map[string]int{},
		sectionMaxScroll:  map[string]int{},
	}
	m.width, m.height = 120, 40
	m.hallCursor = cursorTarget{kind: ui.RowProject, projectID: "p1"}

	m.renderTavernView() // populates paneLinkSpans + geometry
	if len(m.paneLinkSpans) != 1 {
		t.Fatalf("expected 1 recorded link span, got %d", len(m.paneLinkSpans))
	}
	sp := m.paneLinkSpans[0]
	if sp.url != full {
		t.Fatalf("link span url = %q, want the full URL", sp.url)
	}

	clickY := m.rowsScreenTop + (sp.vis - m.scrollOffset)
	click := func() {
		m.handleTavernClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: sp.x0, Y: clickY})
	}

	click() // first click copies (arms the double-click)
	if m.lastLinkClickURL != full {
		t.Fatalf("first click should copy + arm; lastLinkClickURL = %q", m.lastLinkClickURL)
	}
	click() // fast second click opens + disarms
	if m.lastLinkClickURL != "" {
		t.Fatalf("second click should open + disarm; lastLinkClickURL = %q", m.lastLinkClickURL)
	}
}
