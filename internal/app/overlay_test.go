package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Driving the ticker to completion must always empty the particles — the
// ticker can never get "stuck" leaving ghosts frozen on screen.
func TestOverlayDrainsToEmpty(t *testing.T) {
	m := &Model{}
	m.spawnCursorTrail(5, 3)
	cmd := m.maybeStartOverlayTick()
	if cmd == nil {
		t.Fatal("expected a ticker while particles exist")
	}
	gen := m.overlayGen
	// Pump ticks until it stops (bounded).
	for i := 0; i < 1000; i++ {
		if next := m.onOverlayTick(gen); next == nil {
			break
		}
	}
	if len(m.overlayParticles) != 0 {
		t.Fatalf("particles never drained: %d left", len(m.overlayParticles))
	}
}

// A stale-generation tick (from a superseded ticker) must be a clean no-op and
// must NOT touch the live particles — the regression that used to strand the
// "believed running" flag and freeze a cursor ghost on every row.
func TestOverlayStaleTickIsNoOp(t *testing.T) {
	m := &Model{}
	m.spawnCursorTrail(1, 1)
	m.maybeStartOverlayTick() // gen = 1
	staleGen := m.overlayGen
	m.maybeStartOverlayTick() // gen = 2 supersedes
	before := len(m.overlayParticles)
	if next := m.onOverlayTick(staleGen); next != nil {
		t.Fatal("a stale-gen tick must not reschedule")
	}
	if len(m.overlayParticles) != before {
		t.Fatal("a stale-gen tick must not mutate particles")
	}
	// The current-gen ticker still drains normally afterward.
	for i := 0; i < 1000; i++ {
		if m.onOverlayTick(m.overlayGen) == nil {
			break
		}
	}
	if len(m.overlayParticles) != 0 {
		t.Fatalf("current ticker failed to drain: %d left", len(m.overlayParticles))
	}
}

// After draining, a new spawn must be able to restart the ticker — proving the
// lifecycle is self-healing (no stuck flag blocking a restart).
func TestOverlayRestartsAfterEmpty(t *testing.T) {
	m := &Model{}
	m.spawnCursorTrail(0, 0)
	m.maybeStartOverlayTick()
	for i := 0; i < 1000; i++ {
		if m.onOverlayTick(m.overlayGen) == nil {
			break
		}
	}
	m.spawnCursorTrail(2, 2)
	if m.maybeStartOverlayTick() == nil {
		t.Fatal("ticker must restart for a fresh particle after draining")
	}
}

// The mouse wheel must NOT spawn overlay particles — it scrolls the viewport,
// not the cursor. A wheel path that spawned particles without starting the
// ticker is exactly what froze a "›" ghost on every scrolled row.
func TestWheelScrollSpawnsNoTrail(t *testing.T) {
	ui.Init(true)
	st := &store.Store{Projects: []model.Project{{ID: "p1", Name: "C"}}, Quests: []model.Quest{
		{ID: "q1", Title: "one", ProjectID: "p1", Status: model.StatusOpen},
		{ID: "q2", Title: "two", ProjectID: "p1", Status: model.StatusOpen},
	}}
	m := &Model{store: st, wilds: false, collapsedProjects: map[string]bool{}, collapsedSections: map[string]bool{},
		sectionScroll: map[string]int{}, sectionMaxScroll: map[string]int{}}
	m.width, m.height, m.leftColWidth = 120, 40, 40
	m.cursorScreenX, m.cursorScreenY = 4, 3
	// Put the cursor on the first quest of the (campaigns) column.
	rows := m.visibleRows()
	for _, r := range rows {
		if r.Kind == ui.RowQuest {
			m.cursor = targetFromRow(r)
			break
		}
	}
	before := m.cursor
	m.handleWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown}) // a scroll event
	if len(m.overlayParticles) != 0 {
		t.Fatalf("wheel scroll spawned %d particles; must spawn none", len(m.overlayParticles))
	}
	if m.cursor != before {
		t.Fatal("wheel scroll must NOT move the cursor")
	}
	// Keyboard nav, by contrast, DOES move the cursor and leave a trail.
	m.cursorScreenX, m.cursorScreenY = 4, 3
	m.moveCursor(1)
	if len(m.overlayParticles) == 0 {
		t.Fatal("keyboard moveCursor should leave a trail ghost")
	}
}
