package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// sigilModel builds a Model + quest exercising EVERY sigil kind (agent, jira,
// PR, rune, track, lookout) plus a dismissed track, so focusCodeLines emits the
// full set of stops: section headers, items, and all affordances.
func sigilModel() (*Model, *model.Quest) {
	ui.Init(true)
	q := model.Quest{
		ID: "q1", Title: "T", ProjectID: "p1",
		JiraCodes:       []string{"EPDCHAIR-1"},
		PRs:             []model.PRLink{{Code: "#1", Repo: "o/r"}},
		AgentWorkspaces: []string{"w0"},
		Runes:           []string{"flag_a"},
		Tracks:          []model.Track{{Event: "Ev", Marks: []string{"mark"}}},
		Lookouts:        []model.Lookout{{URL: "https://dash/x"}},
		DismissedTracks: []string{"gone"},
	}
	m := &Model{
		store:               &store.Store{Quests: []model.Quest{q}, Projects: []model.Project{{ID: "p1", Name: "Camp"}}},
		integrationsEnabled: true,
		focusLinkIdx:        noSelection,
		modal:               &Modal{Kind: ModalQuestDetail, QuestID: "q1"},
		prStatus:            map[string]PRStatus{},
		jiraStatus:          map[string]JiraStatus{},
		runeStatus:          map[string]RuneStatus{},
		sectionScroll:       map[string]int{},
		sectionMaxScroll:    map[string]int{},
	}
	return m, &m.store.Quests[0]
}

// render populates m.focusLinks / m.focusCodeSpans as a real frame would.
func (m *Model) renderSigils(q *model.Quest) {
	m.focusLinks = nil
	m.focusCodeSpans = nil
	m.focusCodeLines(q, 0, 0)
}

// The invariant that broke Down-nav: the count used to bound navigation MUST
// equal the number of stops actually rendered. If a future change adds/removes
// a stop in focusCodeLines without updating focusLinkCount, this fails loudly.
func TestFocusLinkCountMatchesRender(t *testing.T) {
	m, q := sigilModel()
	m.renderSigils(q)
	if got, want := m.focusLinkCount(q), len(m.focusLinks); got != want {
		t.Fatalf("focusLinkCount=%d but focusCodeLines rendered %d stops — Down-nav will drift", got, want)
	}
	if len(m.focusLinks) == 0 {
		t.Fatal("expected sigil stops to be rendered")
	}
}

// Down from the top must reach the LAST stop (not stop early), and Up must walk
// back to the top — the exact symptom the user reported.
func TestSigilDownUpTraversal(t *testing.T) {
	m, q := sigilModel()
	m.renderSigils(q)
	last := len(m.focusLinks) - 1

	m.focusLinkIdx = 0
	for i := 0; i < len(m.focusLinks)+3; i++ { // over-press: must clamp, not overshoot
		m.handleFocusLinkKey(tea.KeyPressMsg{Code: tea.KeyDown}, q)
	}
	if m.focusLinkIdx != last {
		t.Fatalf("Down settled at %d, want last stop %d", m.focusLinkIdx, last)
	}
	for i := 0; i < len(m.focusLinks); i++ {
		if m.focusLinkIdx == 0 {
			break
		}
		m.handleFocusLinkKey(tea.KeyPressMsg{Code: tea.KeyUp}, q)
	}
	if m.focusLinkIdx != 0 {
		t.Fatalf("Up settled at %d, want 0", m.focusLinkIdx)
	}
}

// Click-to-focus relies on every clickable sigil span sitting on a row that
// resolves back to a focus link. If a span is recorded on a line with no link,
// clicking it can't focus (and Down couldn't re-enter it) — regression guard.
func TestEverySigilSpanResolvesToALink(t *testing.T) {
	m, q := sigilModel()
	m.renderSigils(q)
	for _, sp := range m.focusCodeSpans {
		if m.focusLinkAtLine(sp.line) == noSelection {
			t.Fatalf("span on line %d (url=%q) has no focus link — click can't focus it", sp.line, sp.url)
		}
	}
}

// A render smoke test: viewQuestDetail must not panic across the focus states
// that have historically crashed/regressed (body caret, a focused sigil, an
// open lookout rename). It doesn't assert pixels — just that a frame renders.
func TestViewQuestDetailRenderSmoke(t *testing.T) {
	m, q := sigilModel()
	m.width, m.height = 120, 40
	m.detailWidthRatio = 0.42
	m.modal.BodyEditor = m.newBodyEditor("")
	q.Body = []model.BodyLine{{Text: "a line"}}

	for _, name := range []string{"body", "sigil-focused", "renaming"} {
		switch name {
		case "body":
			m.focusLinkIdx = noSelection
		case "sigil-focused":
			m.focusLinkIdx = 0
		case "renaming":
			m.focusLinkIdx = noSelection
			m.beginLookoutRename("q1", "https://dash/x")
		}
		if out := m.viewQuestDetail(); out == "" {
			t.Fatalf("%s: viewQuestDetail rendered empty", name)
		}
	}
}

// The wheel over the Sigils pane scrolls the pane's VIEWPORT and must never
// move the focus-link cursor (scroll is decoupled from selection everywhere).
func TestWheelSigilsViewportOnly(t *testing.T) {
	m, q := sigilModel()
	m.width, m.height = 120, 60
	m.detailWidthRatio = 0.42
	m.modal.BodyEditor = m.newBodyEditor("")
	q.Body = []model.BodyLine{{Text: "a"}}
	m.focusBodyX = 60                // sigils pane is x < 60
	m.sectionMaxScroll["sigils"] = 5 // pretend the pane overflows
	m.focusLinkIdx = 2

	m.handleFocusWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown}) // over sigils (x=0)
	if m.focusLinkIdx != 2 {
		t.Fatalf("wheel must not move the focus cursor: idx=%d", m.focusLinkIdx)
	}
	if m.sectionScroll["sigils"] != 1 {
		t.Fatalf("wheel should scroll the sigils viewport by 1, got %d", m.sectionScroll["sigils"])
	}
}
