package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func flickerModel(h int) *Model {
	m := &Model{sectionScroll: map[string]int{}, sectionMaxScroll: map[string]int{},
		collapsedProjects: map[string]bool{}, collapsedSections: map[string]bool{},
		newQuestIDs: map[string]bool{}, boxCache: map[string]*boxCacheEntry{},
		railWidthRatio: 0.34, railBoxRatios: []float64{1.0 / 3, 1.0 / 3, 1.0 / 3}}
	m.width, m.height = 100, h
	return m
}

// The rendered frame must be exactly m.height lines on EVERY frame, so scrolling
// a view whose content overflows by a line or two never changes the View height
// (which would make the renderer repaint the whole screen and flicker).
func assertConstantScreenHeight(t *testing.T, label string, m *Model, wheel func()) {
	t.Helper()
	for step := 0; step < 20; step++ {
		frame := m.padToScreen(m.compositeOverlay(m.renderContent()))
		if got := len(strings.Split(frame, "\n")); got != m.height {
			t.Fatalf("%s step=%d: frame is %d lines, want exactly m.height=%d", label, step, got, m.height)
		}
		wheel()
	}
}

func TestScrollFrameHeightConstant(t *testing.T) {
	ui.Init(true)
	const H = 24

	// Quest detail — a long body scrolls the focus view (shares renderFocusView).
	qC := model.Quest{ID: "q1", Title: "Q"}
	for i := 0; i < H-11; i++ { // just past the viewport
		qC.Body = append(qC.Body, model.BodyLine{ID: fmt.Sprintf("b%d", i), Text: fmt.Sprintf("line %d", i)})
	}
	stC := &store.Store{Quests: []model.Quest{qC}}
	mC := flickerModel(H)
	mC.store = stC
	mC.openQuestDetailForTest(&stC.Quests[0])
	assertConstantScreenHeight(t, "quest", mC, func() {
		mC.handleFocusWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	})

	// Section page (shares renderFocusView).
	stS := &store.Store{}
	for i := 0; i < H-9; i++ {
		stS.Quests = append(stS.Quests, model.Quest{ID: fmt.Sprintf("s%d", i), Title: fmt.Sprintf("Q %d", i), Status: model.StatusOpen})
	}
	mS := flickerModel(H)
	mS.store = stS
	mS.modal = sectionDetailModal("questboard")
	if r, ok := nearestSelectableRow(mS.sectionRows("questboard"), 0); ok {
		mS.setCursor(r)
	}
	assertConstantScreenHeight(t, "section", mS, func() {
		mS.handleFocusWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	})

	// Flat Camp list (only active/taken quests show there).
	stW := &store.Store{}
	for i := 0; i < H-9; i++ {
		stW.Quests = append(stW.Quests, model.Quest{ID: fmt.Sprintf("w%d", i), Title: fmt.Sprintf("Q %d", i), Status: model.StatusActive})
	}
	mW := flickerModel(H)
	mW.store = stW
	mW.wilds = true
	if r, ok := nearestSelectableRow(mW.visibleRows(), 0); ok {
		mW.setCursor(r)
	}
	assertConstantScreenHeight(t, "wilds", mW, func() {
		mW.handleWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	})
}

func frame(m *Model) string { return m.padToScreen(m.compositeOverlay(m.renderContent())) }

// Scrolling down while already at the bottom must be a true no-op: the scroll
// offset stays put and the frame is byte-identical (so Bubble Tea emits nothing
// and the screen can't flicker).
func TestWheelNoOpAtBottom(t *testing.T) {
	ui.Init(true)
	const H = 24

	// Campaign/section focus page.
	stF := &store.Store{}
	for i := 0; i < 40; i++ {
		stF.Quests = append(stF.Quests, model.Quest{ID: fmt.Sprintf("q%d", i), Title: fmt.Sprintf("Q %d", i), Status: model.StatusOpen})
	}
	mF := flickerModel(H)
	mF.store = stF
	mF.modal = sectionDetailModal("questboard")
	if r, ok := nearestSelectableRow(mF.sectionRows("questboard"), 0); ok {
		mF.setCursor(r)
	}
	for i := 0; i < 60; i++ { // scroll well past the bottom
		mF.handleFocusWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		_ = frame(mF)
	}
	atBottom := frame(mF)
	bottomScroll := mF.focusScroll
	if bottomScroll != mF.focusScrollMax {
		t.Fatalf("focus not at max: scroll=%d max=%d", bottomScroll, mF.focusScrollMax)
	}
	for i := 0; i < 5; i++ {
		mF.handleFocusWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		if mF.focusScroll != bottomScroll {
			t.Fatalf("focus scroll moved past the bottom: %d -> %d", bottomScroll, mF.focusScroll)
		}
		if frame(mF) != atBottom {
			t.Fatal("focus frame changed while scrolling past the bottom")
		}
	}

}

// padToScreen clips a frame that is taller than the screen and pads a short one,
// always yielding exactly m.height lines.
func TestPadToScreen(t *testing.T) {
	m := &Model{}
	m.height = 5
	if got := len(strings.Split(m.padToScreen("a\nb"), "\n")); got != 5 {
		t.Fatalf("short frame padded to %d lines, want 5", got)
	}
	if got := len(strings.Split(m.padToScreen("a\nb\nc\nd\ne\nf\ng"), "\n")); got != 5 {
		t.Fatalf("tall frame clipped to %d lines, want 5", got)
	}
}
