package app

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/config"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// twoColGap is the number of blank columns between the left rail and the right
// campaigns column — a single column so the hover/drag indicator line sits
// exactly centered between the two boxes (nothing else can occupy the gap to
// push it toward one side).
const twoColGap = 1

// ellipsis marks a section box that has more content scrolled out of view.
const ellipsis = "···"

// tavernTopPad is the blank rows above the TAVERN / WILDS header.
const tavernTopPad = 2

// resizeTarget identifies which draggable divider a drag (or hover) is against.
// Only the quest-detail Sigils/body divider remains (the Tavern's rail dividers
// were retired when it became one room at a time).
type resizeTarget int

const (
	resizeNone      resizeTarget = iota
	resizeDetailCol              // the quest-detail Sigils/body divider (the box's right border)
)

// railBoxCount is the number of stacked boxes in the left rail: Questboard,
// Runes, Wards, Vault — see BuildRailColumn.
const railBoxCount = 4

// normalizeRailRatios coerces a stored ratio list (which older configs held
// as 3 entries) to exactly railBoxCount positive weights summing to 1. Any
// length mismatch or degenerate sum resets to an even split — layout is a
// cosmetic preference, so a reset on a schema change is acceptable and never
// touches quest data.
func normalizeRailRatios(in []float64) []float64 {
	even := func() []float64 {
		out := make([]float64, railBoxCount)
		for i := range out {
			out[i] = 1.0 / float64(railBoxCount)
		}
		return out
	}
	if len(in) != railBoxCount {
		return even()
	}
	sum := 0.0
	for _, r := range in {
		if r < 0 {
			return even()
		}
		sum += r
	}
	if sum <= 0 {
		return even()
	}
	out := make([]float64, railBoxCount)
	for i, r := range in {
		out[i] = r / sum
	}
	return out
}

type resizeDragState struct {
	active bool
	target resizeTarget
}

// updateResizeDrag recomputes the dragged ratio from the CURRENT absolute
// cursor position (not a delta from the last motion event) — coalesced or
// dropped motion events are a non-issue this way: whatever position the
// terminal last delivered, the ratio snaps directly to what it implies, with
// no compounding drift.
func (m *Model) updateResizeDrag(x, _ int) {
	// The only live drag is the quest-detail Sigils/body divider; the Tavern's
	// rail dividers are retired (rooms show one full-width section at a time).
	if m.resizeDrag.target != resizeDetailCol {
		return
	}
	cw := clampInt(m.width-8, 20, 150)
	lm := (m.width - cw) / 2
	if lm < 0 {
		lm = 0
	}
	r := ratioFromColumnDrag(x, lm, cw)
	m.detailWidthRatio = clampFloat(r, 0.22, 0.72)
	m.invalidateRender()
}

// endResizeDrag commits whatever ratio was live-updated during the drag by
// persisting it to config.toml, then clears drag state. Cheap no-op if
// nothing was dragging.
func (m *Model) endResizeDrag() {
	if !m.resizeDrag.active {
		return
	}
	m.resizeDrag = resizeDragState{}
	m.saveLayoutConfig()
}

// resizeStep is how much a single Ctrl+arrow keypress moves a divider ratio.
const resizeStep = 0.04

// resizeColumnWidth grows (delta>0) or shrinks the LEFT section horizontally —
// the quest-detail Sigils pane when a quest is open, else the Tavern's rail
// column. A no-op in views without a horizontal split. Persisted like a drag.
func (m *Model) resizeColumnWidth(delta float64) {
	// Detail view only — the Sigils/body divider. (The Tavern's rail resize is
	// retired; rooms show one full-width section at a time.)
	if m.modal == nil || m.modal.Kind != ModalQuestDetail {
		return
	}
	m.detailWidthRatio = clampFloat(m.detailWidthRatio+delta, 0.22, 0.72)
	m.saveLayoutConfig()
	m.invalidateRender()
}

// saveLayoutConfig persists the current rail/campaigns ratio, rail-box
// ratios, and which sections are collapsed to config.toml (~/.config/quests
// — untouched by reinstalling the app, so this survives one too) —
// best-effort; a write failure (e.g. a read-only filesystem) just means it
// doesn't survive a restart, not worth surfacing to the user mid-drag/click.
func (m *Model) saveLayoutConfig() {
	if m.cfgPath == "" {
		return
	}
	cfg, err := config.Load(m.cfgPath)
	if err != nil {
		return
	}
	cfg.Layout.RailWidthRatio = m.railWidthRatio
	cfg.Layout.RailBoxRatios = m.railBoxRatios
	cfg.Layout.DetailWidthRatio = m.detailWidthRatio
	cfg.Layout.CollapsedSections = m.collapsedSectionsList()
	_ = config.Save(m.cfgPath, cfg)
}

// collapsedSectionsList is m.collapsedSections as a stably-ordered list, for
// persisting to config.toml.
func (m *Model) collapsedSectionsList() []string {
	var out []string
	for _, sec := range [...]string{"inbox", "runes", "lookouts", "someday"} {
		if m.collapsedSections[sec] {
			out = append(out, sec)
		}
	}
	return out
}

// ratioFromColumnDrag turns an absolute cursor X into the rail's desired
// width fraction of tavernWidth — railWidthFor's own clamps enforce the real
// min/max at render time, so this doesn't need to duplicate them.
func ratioFromColumnDrag(x, leftMargin, contentWidth int) float64 {
	if contentWidth <= 0 {
		return 0.34
	}
	railW := x - leftMargin
	return float64(railW) / float64(contentWidth)
}

// minColWidth is the narrowest either the rail or the campaigns column is
// ever allowed to get — same floor both directions, so dragging the column
// divider all the way to either side leaves both sides equally usable
// instead of one having a much higher floor than the other. tavernWidth's own
// 50-column floor comfortably fits both columns at their minimum
// (2*minColWidth+twoColGap = 49).
const minColWidth = 24

func fitWidth(s string, w int) string {
	if w < 0 {
		w = 0
	}
	s = lipgloss.NewStyle().MaxWidth(w).Render(s)
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// sectionColor is a section's accent (full-saturation) color.
func sectionColor(section string) color.Color {
	switch section {
	case "runes":
		return ui.ColorRune
	case "lookouts":
		return ui.ColorLookout
	case "trails":
		return ui.ColorSide // PRs link to GitHub — the blue side-quest accent
	case "someday":
		return ui.ColorRust
	case "campaigns":
		return ui.ColorCampaign
	default: // inbox / questboard
		return ui.ColorAccent
	}
}

// sectionMotif is the little emblem drawn to the right of a section's title.
func sectionMotif(section string) string {
	switch section {
	case "inbox":
		return "\U000f00e5" // nf-md-bulletin_board
	case "runes":
		return "\U000f0b2f" // nf-md-crystal_ball
	case "lookouts":
		return "\U000f0a00" // nf-md-lighthouse_on
	case "trails":
		return "\U000f0d20" // nf-md-map_marker_path — a trail/route (gamier than the git glyph)
	case "someday":
		return "\U000f0726" // nf-md-treasure_chest
	case "campaigns":
		return "\U000f02dc" // nf-md-home (The Hall)
	}
	return ""
}

// drawBox renders a bordered box: title + motif embedded in the top border,
// interior lines (already sized/fit) inside, bottom border. frame styles the
// border runes and motif.
func drawBox(title, motif string, interior []string, colW int, b lipgloss.Border, frame lipgloss.Style) []string {
	innerW := colW - 4
	if innerW < 1 {
		innerW = 1
	}
	tw, mw := lipgloss.Width(title), lipgloss.Width(motif)
	fill := colW - tw - mw - 8 // TL+Top + spaces(4) + Top+TR
	top := frame.Render(b.TopLeft+b.Top) + " " + title + " "
	if fill >= 1 && motif != "" {
		top += frame.Render(strings.Repeat(b.Top, fill)) + " " + frame.Render(motif) + " " + frame.Render(b.Top+b.TopRight)
	} else {
		f := colW - tw - 5
		if f < 0 {
			f = 0
		}
		top += frame.Render(strings.Repeat(b.Top, f) + b.TopRight)
	}
	left, right := frame.Render(b.Left), frame.Render(b.Right)
	lines := []string{top}
	for _, c := range interior {
		lines = append(lines, left+" "+fitWidth(c, innerW)+" "+right)
	}
	lines = append(lines, frame.Render(b.BottomLeft+strings.Repeat(b.Bottom, colW-2)+b.BottomRight))
	return lines
}

// boxCacheEntry is one section's cached wrapped content + clickable spans,
// valid while uiVersion / width / cursor / collapsed are unchanged. Scrolling
// changes none of those, so it lets a scroll reuse the render instead of
// re-wrapping every row (the viewport pattern).
type boxCacheEntry struct {
	uiVersion int
	innerW    int
	cursor    cursorTarget
	collapsed bool
	content   []string
	rows      []int
	hint      map[int][]hintSpan
	code      map[int][]codeSpan
}

// scrollWithMargin keeps idx visible with a one-line margin from the top/bottom
// edges (reserved for the ··· markers), clamped to the content bounds.
func scrollWithMargin(idx, scroll, view, total int) int {
	margin := 1
	if view <= 2*margin+1 {
		margin = 0
	}
	if idx < scroll+margin {
		scroll = idx - margin
	}
	if idx > scroll+view-1-margin {
		scroll = idx - view + 1 + margin
	}
	if max := total - view; scroll > max {
		scroll = max
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

// scrollWindow slices content to areaH lines from scroll, marking the first/
// last line with an ellipsis when content is clipped above/below.
func scrollWindow(content []string, rowIdx []int, scroll, areaH int) ([]string, []int) {
	win := make([]string, areaH)
	winRows := make([]int, areaH)
	for i := 0; i < areaH; i++ {
		src := scroll + i
		if src >= 0 && src < len(content) {
			win[i] = content[src]
			winRows[i] = rowIdx[src]
		} else {
			win[i] = ""
			winRows[i] = -1
		}
	}
	dim := ui.StyleMuted.Render(ellipsis)
	if scroll > 0 && areaH > 0 {
		win[0], winRows[0] = dim, -1
	}
	if scroll+areaH < len(content) && areaH > 0 {
		win[areaH-1], winRows[areaH-1] = dim, -1
	}
	return win, winRows
}
