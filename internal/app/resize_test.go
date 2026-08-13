package app

import (
	"math"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

func sum(xs []float64) float64 {
	t := 0.0
	for _, x := range xs {
		t += x
	}
	return t
}

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

func TestResizeColumnWidthTavernRail(t *testing.T) {
	m := &Model{railWidthRatio: 0.34} // no modal, not wilds/search → twoColumn()
	m.resizeColumnWidth(resizeStep)
	if m.railWidthRatio <= 0.34 {
		t.Fatalf("Ctrl+→ should grow the rail: %v", m.railWidthRatio)
	}
}

func TestResizeRailBox(t *testing.T) {
	r := []float64{0.25, 0.25, 0.25, 0.25}
	if !resizeRailBox(r, 1, resizeStep) {
		t.Fatal("expected a change")
	}
	if math.Abs(sum(r)-1) > 1e-9 {
		t.Fatalf("weights must still sum to 1: %v (sum %v)", r, sum(r))
	}
	if r[1] <= 0.25 {
		t.Fatalf("box 1 should grow: %v", r)
	}

	// The stuck case from the bug report: inbox squished next to another squished
	// box, most weight hoarded by box 2. Growing inbox MUST actually grow it
	// (by taking from the largest box), not reverse.
	r = []float64{0.058, 0.058, 0.846, 0.038}
	before := r[0]
	if !resizeRailBox(r, 0, resizeStep) {
		t.Fatal("growing a squished box should change ratios")
	}
	if r[0] <= before {
		t.Fatalf("squished inbox must grow, got %v (was %v)", r[0], before)
	}
	if r[2] >= 0.846 {
		t.Fatalf("the largest box should have donated the room: %v", r)
	}
	if math.Abs(sum(r)-1) > 1e-9 {
		t.Fatalf("sum must stay 1: %v", r)
	}
	// Out-of-range index is a safe no-op.
	if resizeRailBox(r, -1, resizeStep) || resizeRailBox(r, 9, resizeStep) {
		t.Fatal("out-of-range index should not change anything")
	}
}

func TestDetailResizeKeysWhenSigilFocused(t *testing.T) {
	m, q := sigilModel()
	m.width, m.height = 120, 40
	m.detailWidthRatio = 0.42
	m.modal.BodyEditor = m.newBodyEditor("")
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
	m.modal.BodyCursor = 0
	m.modal.BodyEditor = m.newBodyEditor("one")
	m.focusLinkIdx = noSelection // in the body, NOT Sigils

	// Alt+Down in the body must NOT resize (it's move-line there).
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt})
	if m.detailWidthRatio != 0.42 {
		t.Fatalf("alt+down in the body should not resize: %v", m.detailWidthRatio)
	}
}
