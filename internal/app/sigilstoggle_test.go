package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The compact detail view shows only connection sections that have value; empty
// ones stay hidden (revealable with F3) — even a section that's mid-find but
// still empty. sigilsHaveHidden reports whether there's anything to reveal.
func TestSigilsHiddenGating(t *testing.T) {
	ui.Init(true)
	m := &Model{store: &store.Store{}}

	// A bare quest: every section empty → all hideable.
	bare := &model.Quest{ID: "q", Title: "Bare"}
	if !m.sigilsHaveHidden(bare) {
		t.Fatal("a quest with no connections has hidden (empty) sigils to reveal")
	}
	// Compact view: no empty section renders (only the toggle line + NPCs, which
	// always shows). No Scrolls/Runes/Tracks/Lookouts headers.
	m.showHiddenSigils = false
	compact := strings.Join(m.focusCodeLines(bare, 0, 0), "\n")
	for _, name := range []string{"Scrolls", "Runes", "Tracks", "Lookouts"} {
		if strings.Contains(compact, name) {
			t.Errorf("compact view should hide the empty %s section", name)
		}
	}
	if !strings.Contains(compact, "show hidden sigils") {
		t.Error("compact view should offer to reveal hidden sigils")
	}

	// Revealed: empty sections show with their hints.
	m.showHiddenSigils = true
	shown := strings.Join(m.focusCodeLines(bare, 0, 0), "\n")
	for _, want := range []string{"Scrolls", "paste a Jira link", "Runes", "found in your trails", "Lookouts"} {
		if !strings.Contains(shown, want) {
			t.Errorf("revealed view should show %q", want)
		}
	}

	// A quest that's mid-find but still empty stays hidden in the compact view.
	m.showHiddenSigils = false
	m.findingQuestID = "q"
	compact2 := strings.Join(m.focusCodeLines(bare, 0, 0), "\n")
	if strings.Contains(compact2, "finding") {
		t.Error("a loading-but-empty section must still be hidden in the compact view")
	}
}

// Clicking the "show/hide hidden sigils" line toggles it — the mouse twin of F3,
// not just the hotkey.
func TestClickTogglesHiddenSigils(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store:  &store.Store{Quests: []model.Quest{{ID: "q", Title: "Bare"}}},
		modal:  &Modal{Kind: ModalQuestDetail, QuestID: "q"},
		width:  120,
		height: 40,
	}
	q := &m.store.Quests[0]

	// Render the sigils to register the click span (baseX 0 → span at gutter col).
	m.showHiddenSigils = false
	m.focusCodeLines(q, 0, 0)

	var span focusCodeSpan
	found := false
	for _, sp := range m.focusCodeSpans {
		if sp.url == toggleSigilsSentinel {
			span, found = sp, true
		}
	}
	if !found {
		t.Fatal("the show-hidden-sigils line should register a clickable span")
	}

	// Neutralize the header/title click zones, then click the toggle span.
	m.focusHeaderRow = -1
	m.focusTitleWidth = 0
	m.focusContentTop = 0
	mouse := tea.Mouse{X: span.x0, Y: m.focusContentTop + span.line, Button: tea.MouseLeft}
	m.handleFocusPointer(mouse, true)

	if !m.showHiddenSigils {
		t.Fatal("clicking the toggle line should reveal hidden sigils")
	}
}
