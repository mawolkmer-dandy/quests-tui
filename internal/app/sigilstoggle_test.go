package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// F3 in the quest detail reveals the empty connection sections, and toggles them
// back off — but only matters when a section is actually empty (something to
// reveal).
func TestF3TogglesHiddenSigils(t *testing.T) {
	m, q := sigilModel()
	m.width, m.height = 120, 40
	m.modal.BodyEditor = m.newBodyEditor("")
	q.Body = []model.BodyLine{{Text: "a"}}
	// Strip every hideable connection so the sections are empty (NPCs stays —
	// it always shows). Now there's something for F3 to reveal.
	q.JiraCodes, q.PRs, q.Runes, q.Tracks, q.Lookouts, q.DismissedTracks = nil, nil, nil, nil, nil, nil

	if !m.sigilsHaveHidden(q) {
		t.Fatal("a quest with no links should have hidden (empty) sigils to reveal")
	}
	if m.showHiddenSigils {
		t.Fatal("empty sigils should start collapsed")
	}
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyF3})
	if !m.showHiddenSigils {
		t.Fatal("F3 should reveal the hidden sigils")
	}
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyF3})
	if m.showHiddenSigils {
		t.Fatal("F3 again should collapse them")
	}
}
