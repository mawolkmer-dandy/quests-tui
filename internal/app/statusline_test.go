package app

import (
	"strings"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The focused sigil's action hint must appear ONCE, on the fixed status line
// below the box — never inline on the row (which used to wrap and shift the
// whole layout). Checked for every focusable stop.
func TestSigilHintOnStatusLineNotInline(t *testing.T) {
	m, q := sigilModel()
	m.width, m.height = 120, 60 // tall enough that nothing scrolls out
	m.detailWidthRatio = 0.42
	m.bodyEditor = m.newBodyEditor("")
	q.Body = []model.BodyLine{{Text: "a"}}

	for i := range m.focusLinks {
		m.focusLinkIdx = i
		out := stripANSI(m.viewQuestDetail())
		lines := strings.Split(out, "\n")
		// The status line is the last non-empty line, OUTSIDE the box borders.
		var status string
		for j := len(lines) - 1; j >= 0; j-- {
			if strings.TrimSpace(lines[j]) != "" {
				status = lines[j]
				break
			}
		}
		if strings.ContainsAny(status, "│╰╭") {
			t.Fatalf("stop %d: status text landed inside the box, not on its own line:\n%s", i, out)
		}
	}
}

func TestSigilStatusEmptyWhenBodyFocused(t *testing.T) {
	m, q := sigilModel()
	m.width, m.height = 120, 60
	m.detailWidthRatio = 0.42
	m.bodyEditor = m.newBodyEditor("")
	q.Body = []model.BodyLine{{Text: "hello"}}
	m.focusLinkIdx = noSelection

	out := stripANSI(m.viewQuestDetail())
	if strings.Contains(out, "to open") || strings.Contains(out, "to copy") {
		t.Fatalf("no status hint expected when the body owns focus:\n%s", out)
	}
}

// The status-line decision applies to EVERY view, not just the quest detail.
// statusHint resolves the cursor row's actions in the Tavern/outline and the
// focus pages alike — this is the "apply it everywhere" invariant.
func TestStatusHintAcrossViews(t *testing.T) {
	ui.Init(true)
	st := &store.Store{
		Projects: []model.Project{{ID: "p1", Name: "Camp"}},
		Quests:   []model.Quest{{ID: "q1", Title: "A quest", ProjectID: "p1", Status: model.StatusOpen}},
	}
	m := &Model{
		store:             st,
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{},
	}

	// Outline: cursor on a quest → the quest's "open" hint on the status line.
	m.cursor = cursorTarget{kind: ui.RowQuest, questID: "q1", projectID: "p1"}
	if got := stripANSI(m.statusHint()); !strings.Contains(got, "open") {
		t.Fatalf("outline status hint for a quest = %q, want an open hint", got)
	}
	// Same row, hints toggled off → empty (Ctrl+K).
	m.hideHoverTips = true
	if got := m.statusHint(); got != "" {
		t.Fatalf("hideHoverTips should silence the status hint, got %q", got)
	}
	m.hideHoverTips = false

	// Section focus page: cursor on a rune row → open + copy.
	st.Quests[0].Runes = []string{"flag_a"}
	m.modal = &Modal{Kind: ModalSectionDetail, Section: "runes"}
	rows := m.sectionRows("runes")
	for _, r := range rows {
		if r.Kind == ui.RowRune {
			m.cursor = targetFromRow(r)
			break
		}
	}
	if got := stripANSI(m.statusHint()); !strings.Contains(got, "copy") {
		t.Fatalf("section-page rune status hint = %q, want a copy hint", got)
	}
}

// Hint language is uniform: "<key> to <verb>" everywhere, keys styled.
func TestHintLanguageFormat(t *testing.T) {
	ui.Init(true)
	// Outline quest → "tab to open".
	got := stripANSI(renderHintParts(actionHintParts(ui.Row{Kind: ui.RowQuest})))
	if !strings.Contains(got, "tab to open") {
		t.Fatalf("quest hint = %q, want 'tab to open'", got)
	}
	// Collapsible → "enter to collapse" / "enter to expand".
	if got := stripANSI(renderHintParts(actionHintParts(ui.Row{Kind: ui.RowSection}))); !strings.Contains(got, "enter to collapse") {
		t.Fatalf("section hint = %q", got)
	}
	if got := stripANSI(renderHintParts(actionHintParts(ui.Row{Kind: ui.RowSection, Collapsed: true}))); !strings.Contains(got, "enter to expand") {
		t.Fatalf("collapsed section hint = %q", got)
	}
	// Lookout → open / copy / rename, all "to"-phrased.
	got = stripANSI(renderHintParts(actionHintParts(ui.Row{Kind: ui.RowLookout})))
	for _, want := range []string{"enter to open", "c to copy", "r to rename"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lookout hint %q missing %q", got, want)
		}
	}
	// The key text is styled (not the same as plain muted) — StyleKey applied.
	if ui.StyleKey.Render("enter") == ui.StyleMuted.Render("enter") {
		t.Fatal("keys should have a distinct style from the muted verb")
	}
}
