package lab

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Color palette indices for grid cells — kept as small ints so a cell is cheap
// and runs of the same color can be grouped into one styled Render call.
const (
	cDefault int8 = iota
	cMuted
	cGreen // done / success
	cGold  // accent / shimmer
	cRed   // blocked / important
	cName  // plain foreground (quest title)
	cRust  // vault / aged
)

func palette(c int8) lipgloss.Style {
	switch c {
	case cMuted:
		return ui.StyleMuted
	case cGreen:
		return lipgloss.NewStyle().Foreground(ui.ColorHeading)
	case cGold:
		return lipgloss.NewStyle().Foreground(ui.ColorAccent)
	case cRed:
		return lipgloss.NewStyle().Foreground(ui.ColorImportant)
	case cName:
		return ui.StyleName
	case cRust:
		return lipgloss.NewStyle().Foreground(ui.ColorRust)
	default:
		return lipgloss.NewStyle()
	}
}

// cell is one character of the preview stage plus its palette color.
type cell struct {
	r rune
	c int8
}

// grid is a small fixed-size character canvas the effects paint onto, so
// particles / rings / sweeps can overlay text freely by (x,y).
type grid struct {
	w, h  int
	cells []cell
}

func newGrid(w, h int) *grid {
	g := &grid{w: w, h: h, cells: make([]cell, w*h)}
	for i := range g.cells {
		g.cells[i] = cell{r: ' ', c: cDefault}
	}
	return g
}

func (g *grid) set(x, y int, r rune, c int8) {
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return
	}
	g.cells[y*g.w+x] = cell{r: r, c: c}
}

// text writes s starting at (x,y), left to right, in color c.
func (g *grid) text(x, y int, s string, c int8) {
	for _, r := range s {
		g.set(x, y, r, c)
		x++
	}
}

// render joins the grid into styled rows, grouping consecutive same-color runs
// into a single Render call so the frame isn't a wall of per-cell escapes.
func (g *grid) render() []string {
	out := make([]string, g.h)
	for y := 0; y < g.h; y++ {
		var b strings.Builder
		var run strings.Builder
		runColor := int8(-1)
		flush := func() {
			if run.Len() == 0 {
				return
			}
			b.WriteString(palette(runColor).Render(run.String()))
			run.Reset()
		}
		for x := 0; x < g.w; x++ {
			cl := g.cells[y*g.w+x]
			if cl.c != runColor {
				flush()
				runColor = cl.c
			}
			run.WriteRune(cl.r)
		}
		flush()
		out[y] = b.String()
	}
	return out
}
