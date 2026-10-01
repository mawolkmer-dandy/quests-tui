package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

func TestResizeColumnWidthDetailClamps(t *testing.T) {
	m := &Model{modal: &Modal{Kind: ModalQuestDetail}, detailWidthRatio: 0.42}
	m.resizeColumnWidth(resizeStep)
	if m.detailWidthRatio <= 0.42 {
		t.Fatalf("Ctrl+→ should grow Sigils: %v", m.detailWidthRatio)
	}
	for i := 0; i < 50; i++ {
		m.resizeColumnWidth(resizeStep)
	}
	if m.detailWidthRatio > 0.72+1e-9 {
		t.Fatalf("detailWidthRatio overshot clamp: %v", m.detailWidthRatio)
	}
	for i := 0; i < 50; i++ {
		m.resizeColumnWidth(-resizeStep)
	}
	if m.detailWidthRatio < 0.22-1e-9 {
		t.Fatalf("detailWidthRatio undershot clamp: %v", m.detailWidthRatio)
	}
}

func TestDetailResizeKeysWhenSigilFocused(t *testing.T) {
	m, q := sigilModel()
	m.width, m.height = 120, 40
	m.detailWidthRatio = 0.42
	m.bodyEditor = m.newBodyEditor("")
	q.Body = []model.BodyLine{{Text: "a"}}
	m.focusLinkIdx = 0 // focused in Sigils

	for _, tc := range []struct {
		key  tea.KeyPressMsg
		grow bool
	}{
		{tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt}, true},
		{tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt}, false},
		{tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt}, true},
		{tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt}, false},
	} {
		m.detailWidthRatio = 0.42
		m.updateModal(tc.key)
		if tc.grow && m.detailWidthRatio <= 0.42 {
			t.Fatalf("%s should grow Sigils, got %v", tc.key.String(), m.detailWidthRatio)
		}
		if !tc.grow && m.detailWidthRatio >= 0.42 {
			t.Fatalf("%s should shrink Sigils, got %v", tc.key.String(), m.detailWidthRatio)
		}
	}
}

func TestDetailAltUpDownMovesLineInBody(t *testing.T) {
	m, q := sigilModel()
	m.width, m.height = 120, 40
	m.detailWidthRatio = 0.42
	q.Body = []model.BodyLine{{Text: "one"}, {Text: "two"}}
	m.bodyCursor = 0
	m.bodyEditor = m.newBodyEditor("one")
	m.focusLinkIdx = noSelection // in the body, NOT Sigils

	// Alt+Down in the body must NOT resize (it's move-line there).
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt})
	if m.detailWidthRatio != 0.42 {
		t.Fatalf("alt+down in the body should not resize: %v", m.detailWidthRatio)
	}
}
