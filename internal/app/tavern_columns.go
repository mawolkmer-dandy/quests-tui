package app

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
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

// resizeTarget identifies which two-column Tavern divider a drag (or hover)
// is against.
type resizeTarget int

const (
	resizeNone    resizeTarget = iota
	resizeColumns              // vertical divider, rail vs. campaigns
	resizeRail0                // between rail box 0 (Questboard) and box 1 (Runes)
	resizeRail1                // between rail box 1 (Runes) and box 2 (Vault)
)

type resizeDragState struct {
	active bool
	target resizeTarget
}

// tryStartResizeDrag checks whether (x,y) lands on a divider's grab cell; if
// so it arms resizeDrag and returns (nil, true) — fully consumed, no further
// click dispatch this event. Only live in the two-column Tavern.
func (m *Model) tryStartResizeDrag(x, y int) (tea.Cmd, bool) {
	if !m.twoColumn() {
		return nil, false
	}
	if t := m.hitTestDivider(x, y); t != resizeNone {
		m.resizeDrag = resizeDragState{active: true, target: t}
		return nil, true
	}
	return nil, false
}

// updateResizeDrag recomputes the dragged ratio from the CURRENT absolute
// cursor position (not a delta from the last motion event) — coalesced or
// dropped motion events are a non-issue this way: whatever position the
// terminal last delivered, the ratio snaps directly to what it implies, with
// no compounding drift.
func (m *Model) updateResizeDrag(x, y int) {
	switch m.resizeDrag.target {
	case resizeColumns:
		m.railWidthRatio = ratioFromColumnDrag(x, m.leftMargin, m.tavernWidth())
	case resizeRail0, resizeRail1:
		m.railBoxRatios = ratioFromRailDrag(m.resizeDrag.target, y, m.rowsScreenTop, m.lastRailHeights)
	}
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
	cfg.Layout.CollapsedSections = m.collapsedSectionsList()
	_ = config.Save(m.cfgPath, cfg)
}

// collapsedSectionsList is m.collapsedSections as a stably-ordered list, for
// persisting to config.toml.
func (m *Model) collapsedSectionsList() []string {
	var out []string
	for _, sec := range [...]string{"inbox", "runes", "someday"} {
		if m.collapsedSections[sec] {
			out = append(out, sec)
		}
	}
	return out
}

// hitTestDivider reports which divider (if any) screen cell (x,y) lands on.
func (m *Model) hitTestDivider(x, y int) resizeTarget {
	gapX0 := m.leftMargin + m.leftColWidth
	if x >= gapX0 && x <= gapX0+twoColGap-1 {
		top := m.rowsScreenTop
		if y >= top && y < top+m.lastViewHeight {
			return resizeColumns
		}
		return resizeNone
	}
	if x < m.leftMargin || x >= m.leftMargin+m.leftColWidth {
		return resizeNone // not over the rail column at all
	}
	return m.hitTestRailDivider(y)
}

// hitTestRailDivider finds which of the 2 internal rail dividers (if any)
// screen row y lands on. Stacked boxes' borders are directly adjacent (no
// gap row — see renderRail), so each divider's unambiguous grab target is
// simply the box ABOVE it's own bottom border row (never the box below's top
// border, which also carries its title text).
func (m *Model) hitTestRailDivider(y int) resizeTarget {
	off := y - m.rowsScreenTop
	heights := m.lastRailHeights
	switch off {
	case heights[0] - 1:
		return resizeRail0
	case heights[0] + heights[1] - 1:
		return resizeRail1
	}
	return resizeNone
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

// ratioFromRailDrag turns an absolute cursor Y into new ratios for all 3 rail
// boxes: the dragged divider moves the boundary between its two neighbors,
// the third box keeps its current absolute share. minH is reserved for BOTH
// neighbors directly in the boundary clamp (not just floored afterward) —
// dragging past the point where a neighbor would go under minH has to stop
// the boundary there, or the far side of the clamp silently keeps growing
// disconnected from the cursor once the near side hits its floor (the "drags
// past the limit and grows the wrong box" bug).
func ratioFromRailDrag(target resizeTarget, y, rowsScreenTop int, heights [3]int) [3]float64 {
	const minH = 4
	total := heights[0] + heights[1] + heights[2]
	if total <= 0 {
		return [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3}
	}
	off := clampInt(y-rowsScreenTop, 0, total)

	h := heights
	switch target {
	case resizeRail0:
		combined := heights[0] + heights[1]
		boundary := splitBoundary(off, combined, minH)
		h[0] = boundary
		h[1] = combined - boundary
	case resizeRail1:
		combined := heights[1] + heights[2]
		boundary := splitBoundary(off-heights[0], combined, minH)
		h[1] = boundary
		h[2] = combined - boundary
	}
	sum := float64(h[0] + h[1] + h[2])
	if sum <= 0 {
		return [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3}
	}
	return [3]float64{float64(h[0]) / sum, float64(h[1]) / sum, float64(h[2]) / sum}
}

// splitBoundary clamps a proposed split point of `combined` rows so BOTH
// resulting sides keep at least minH — the point past which further dragging
// has no more room to give, so it simply stops there instead of one side
// silently continuing to grow.
func splitBoundary(off, combined, minH int) int {
	if combined < 2*minH {
		return combined / 2 // not enough room for both minimums — split evenly
	}
	return clampInt(off, minH, combined-minH)
}

func (m *Model) tavernWidth() int {
	w := m.width - 6
	if w > 220 {
		w = 220
	}
	if w < 50 {
		w = 50
	}
	return w
}

// minColWidth is the narrowest either the rail or the campaigns column is
// ever allowed to get — same floor both directions, so dragging the column
// divider all the way to either side leaves both sides equally usable
// instead of one having a much higher floor than the other. tavernWidth's own
// 50-column floor comfortably fits both columns at their minimum
// (2*minColWidth+twoColGap = 49).
const minColWidth = 24

// railWidthFor sizes the left rail from the user's stored ratio (draggable —
// see hitTestDivider/updateResizeDrag); the wider campaigns list takes the
// rest. Both columns share the same minColWidth floor.
func railWidthFor(contentWidth int, ratio float64) int {
	rail := int(float64(contentWidth)*ratio + 0.5)
	maxRail := contentWidth - twoColGap - minColWidth
	if maxRail < minColWidth {
		maxRail = minColWidth
	}
	return clampInt(rail, minColWidth, maxRail)
}

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

func scrollFor(idx, scroll, view, total int) int {
	if idx >= 0 {
		if idx < scroll {
			scroll = idx
		}
		if idx >= scroll+view {
			scroll = idx - view + 1
		}
	}
	if max := total - view; scroll > max {
		scroll = max
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

func lineFrom(lineMap []int, off int) int {
	if off < 0 || off >= len(lineMap) {
		return -1
	}
	return lineMap[off]
}

// firstOffset inverts a line table: the first on-screen line offset that maps
// to row index idx, or -1 if the row isn't currently on screen.
func firstOffset(lineMap []int, idx int) int {
	for off, r := range lineMap {
		if r == idx {
			return off
		}
	}
	return -1
}

func indexOfInt(s []int, v int) int {
	if v < 0 {
		return -1
	}
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func (m *Model) sectionLabelCount(section string) (string, int) {
	switch section {
	case "inbox":
		return "Questboard", ui.CountInbox(m.store)
	case "runes":
		return "Runes", ui.CountRunes(m.store)
	case "someday":
		return "Vault", ui.CountSomeday(m.store) + ui.CountArchived(m.store)
	}
	return section, 0
}

// sectionColor is a section's accent (full-saturation) color.
func sectionColor(section string) color.Color {
	switch section {
	case "runes":
		return ui.ColorRune
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
	case "someday":
		return "\U000f0726" // nf-md-treasure_chest
	case "campaigns":
		return "\U000f023b" // nf-md-flag
	}
	return ""
}

func sectionBorder(section string) lipgloss.Border {
	if section == "someday" {
		return lipgloss.DoubleBorder()
	}
	return lipgloss.RoundedBorder()
}

// frameStyle is the border/motif style for a section: full accent when active
// (cursor inside), a faint (muted) version otherwise.
func frameStyle(section string, active bool) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(sectionColor(section))
	if !active {
		s = s.Faint(true)
	}
	return s
}

// boxTitle is the caret + label (+ count) placed in a box's top border. Muted
// gray normally, bright white when the section is active.
func (m *Model) boxTitle(section string, header ui.Row, active bool) string {
	var body string
	switch header.Kind {
	case ui.RowLabel:
		body = header.Label
	case ui.RowSection:
		label, count := m.sectionLabelCount(section)
		body = fmt.Sprintf("%s (%d)", label, count)
	}
	caret := ui.GlyphExpanded
	if header.Collapsed {
		caret = ui.GlyphCollapsed
	}
	text := caret + " " + body
	if active {
		return ui.StyleTitle.Render(text)
	}
	return ui.StyleMuted.Render(text)
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

// wrapItems renders item rows [start,end) and word-wraps each to innerW,
// returning the flat wrapped lines and the row index each maps to (-1 for
// spacers/day headers). withSpans uses the span-recording outline renderer
// (campaigns); otherwise the plain box-item renderer (rail).
func (m *Model) wrapItems(rows []ui.Row, start, end, activeIdx, innerW, xBase int, withSpans bool) ([]string, []int) {
	var lines []string
	var lineRows []int
	for i := start; i < end; i++ {
		if rows[i].Kind == ui.RowSpacer {
			lines = append(lines, "")
			lineRows = append(lineRows, -1)
			continue
		}
		var raw string
		if withSpans {
			raw = m.renderOutlineRowLine(rows, i, activeIdx, -1, -1, innerW, xBase)
		} else {
			raw = m.renderBoxItemLine(rows, i, activeIdx, innerW)
		}
		for _, sub := range strings.Split(lipgloss.NewStyle().Width(innerW).Render(raw), "\n") {
			lines = append(lines, sub)
			lineRows = append(lineRows, i)
		}
	}
	return lines, lineRows
}

// renderBoxItemLine renders one item row inside a rail box, cursor mark only
// when it's the active column's cursor.
func (m *Model) renderBoxItemLine(rows []ui.Row, i, activeIdx, innerW int) string {
	row := rows[i]
	isCursor := i == activeIdx
	titleView := ""
	if isCursor && m.editor != nil {
		titleView = m.renderEditableStyled(m.editor, m.cursorTitleStyle(row))
	} else if row.Kind == ui.RowRune {
		titleView = m.runeRowContent(row.RuneKey)
	}
	hint := ""
	if isCursor && m.confirmDeleteID != "" && rowMatchesConfirmDelete(row, m.confirmDeleteID) {
		hint = "  " + ui.StyleImportant.Render(m.confirmDeleteHint(row))
	}
	line, _ := ui.RenderRow(row, m.store, titleView, isCursor, innerW, hint)
	return line
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

// boxContent word-wraps a section's item rows once (empty when collapsed),
// caching the result (and any clickable spans) so a scroll reuses it. On a
// cache hit the cached spans are replayed into the current frame's span maps.
func (m *Model) boxContent(section string, rows []ui.Row, headerIdx, itemStart, itemEnd, activeIdx, innerW, xBase int, withSpans bool) ([]string, []int) {
	collapsed := rows[headerIdx].Collapsed
	// The section holding the cursor carries the LIVE row editor (its value
	// isn't part of the cache key), so it must render fresh every frame or
	// typing/blinking wouldn't show — a stale cache hit is exactly what made
	// typing feel frozen. It's just one section and ~1ms, so no caching there.
	hasCursor := activeIdx >= headerIdx && activeIdx < itemEnd
	if !hasCursor {
		if e := m.boxCache[section]; e != nil && e.uiVersion == m.uiVersion && e.innerW == innerW && e.cursor == m.cursor && e.collapsed == collapsed {
			for k, v := range e.hint {
				m.hintSpans[k] = v
			}
			for k, v := range e.code {
				m.codeSpans[k] = v
			}
			return e.content, e.rows
		}
	}

	// Miss (or the cursor's live section): render into fresh span maps so we can
	// cache exactly this section's
	// spans, then merge them back into the frame's maps.
	prevHint, prevCode := m.hintSpans, m.codeSpans
	m.hintSpans, m.codeSpans = map[int][]hintSpan{}, map[int][]codeSpan{}
	var content []string
	var contentRows []int
	if !collapsed {
		content, contentRows = m.wrapItems(rows, itemStart, itemEnd, activeIdx, innerW, xBase, withSpans)
	}
	e := &boxCacheEntry{
		uiVersion: m.uiVersion, innerW: innerW, cursor: m.cursor, collapsed: collapsed,
		content: content, rows: contentRows, hint: m.hintSpans, code: m.codeSpans,
	}
	m.boxCache[section] = e
	for k, v := range m.hintSpans {
		prevHint[k] = v
	}
	for k, v := range m.codeSpans {
		prevCode[k] = v
	}
	m.hintSpans, m.codeSpans = prevHint, prevCode
	return content, contentRows
}

// sectionTitleWithHint is a section box's title plus, when the section is the
// cursor's or the mouse is hovering its title, the open/collapse hint — the
// fuller hint if it fits the box, otherwise a compact "↵ open", otherwise none.
func (m *Model) sectionTitleWithHint(section string, header ui.Row, active bool, colW int) string {
	title := m.boxTitle(section, header, active)
	if m.hideHoverTips || !(active || m.hoverSection == section) {
		return title
	}
	avail := colW - lipgloss.Width(title) - 6 // corners + spaces + motif budget
	if full := renderHintParts(actionHintParts(header)); lipgloss.Width(full) <= avail {
		return title + full
	}
	if short := "  " + ui.StyleMuted.Render("↵ open"); lipgloss.Width(short) <= avail {
		return title + short
	}
	return title
}

// updateSectionHover sets hoverSection when the mouse is over a section title
// (so its box shows the open/collapse hint), else clears it.
func (m *Model) updateSectionHover(msg tea.Mouse) {
	m.hoverSection = ""
	off := msg.Y - m.rowsScreenTop
	if msg.X >= m.leftMargin+m.leftColWidth+twoColGap {
		rows := m.campaignColumnRows()
		if idx := lineFrom(m.campLineRow, off); idx >= 0 && idx < len(rows) && rows[idx].Kind == ui.RowLabel {
			m.hoverSection = "campaigns"
		}
		return
	}
	rows := m.railColumnRows()
	if idx := lineFrom(m.railLineRow, off); idx >= 0 && idx < len(rows) && rows[idx].Kind == ui.RowSection {
		m.hoverSection = rows[idx].Section
	}
}

// boxNaturalHeight is how tall a box wants to be to show all its content.
func boxNaturalHeight(collapsed bool, contentLen int, vpad bool) int {
	if collapsed {
		return 2 // titled top border + bottom border
	}
	h := contentLen + 2
	if vpad {
		h += 2
	}
	return h
}

// assembleBox renders one section box of exactly `height` lines from
// already-wrapped content: title in the top border, content scrolled to keep
// the cursor visible with ··· markers when clipped, optional vertical padding.
// It also records the box's max scroll so a mouse wheel can scroll it directly.
func (m *Model) assembleBox(section string, header ui.Row, headerIdx, activeIdx int, content []string, contentRows []int, colW, height int, vpad bool, reveal float64) ([]string, []int) {
	active := indexOfInt(contentRows, activeIdx) >= 0 || headerIdx == activeIdx
	title := m.sectionTitleWithHint(section, header, active, colW)

	if header.Collapsed {
		m.sectionMaxScroll[section] = 0
		box := drawBox(title, sectionMotif(section), nil, colW, sectionBorder(section), frameStyle(section, active))
		return box, []int{headerIdx, -1}
	}

	interiorH := height - 2
	if interiorH < 1 {
		interiorH = 1
	}
	padT, padB := 0, 0
	if vpad && interiorH >= 3 {
		padT, padB = 1, 1
	}
	areaH := interiorH - padT - padB
	if areaH < 1 {
		areaH = 1
	}
	if max := len(content) - areaH; max > 0 {
		m.sectionMaxScroll[section] = max
	} else {
		m.sectionMaxScroll[section] = 0
	}

	var win []string
	var winRows []int
	if reveal < 1 {
		// Reveal animation: show only the first fraction of content, so the text
		// fills in line-by-line per section. No scroll while revealing.
		n := int(reveal*float64(len(content)) + 0.999)
		if n > len(content) {
			n = len(content)
		}
		win, winRows = scrollWindow(content[:n], contentRows[:n], 0, areaH)
	} else {
		// Honor the stored scroll (which the mouse wheel sets directly). Only
		// follow the cursor right after a keyboard move — and keep it one line
		// off the ellipsis edges so it never lands on a "···" marker. This
		// decouples wheel scrolling from the cursor (the viewport pattern).
		scroll := m.sectionScroll[section]
		if m.cursorMoved {
			if cursorLine := indexOfInt(contentRows, activeIdx); cursorLine >= 0 {
				scroll = scrollWithMargin(cursorLine, scroll, areaH, len(content))
			}
		}
		if scroll > m.sectionMaxScroll[section] {
			scroll = m.sectionMaxScroll[section]
		}
		if scroll < 0 {
			scroll = 0
		}
		m.sectionScroll[section] = scroll
		win, winRows = scrollWindow(content, contentRows, scroll, areaH)
	}

	var interior []string
	var interiorRows []int
	for k := 0; k < padT; k++ {
		interior = append(interior, "")
		interiorRows = append(interiorRows, -1)
	}
	interior = append(interior, win...)
	interiorRows = append(interiorRows, winRows...)
	for k := 0; k < padB; k++ {
		interior = append(interior, "")
		interiorRows = append(interiorRows, -1)
	}

	box := drawBox(title, sectionMotif(section), interior, colW, sectionBorder(section), frameStyle(section, active))

	lineMap := make([]int, len(box))
	for k := range lineMap {
		lineMap[k] = -1
	}
	lineMap[0] = headerIdx
	for k, r := range interiorRows {
		if 1+k < len(lineMap)-1 {
			lineMap[1+k] = r
		}
	}
	return box, lineMap
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

// sectionSpans splits a rail row list into (headerIdx, itemStart, itemEnd) per
// RowSection.
func sectionSpans(rows []ui.Row) [][3]int {
	var spans [][3]int
	for i := 0; i < len(rows); {
		if rows[i].Kind != ui.RowSection {
			i++
			continue
		}
		j := i + 1
		for j < len(rows) && rows[j].Kind != ui.RowSection {
			j++
		}
		spans = append(spans, [3]int{i, i + 1, j})
		i = j
	}
	return spans
}

// viewTwoColumn renders the Tavern with no vertical padding: a left rail of
// framed Questboard / Runes / Vault scroll-boxes and a right campaigns
// scroll-box. Every section is a box with its title in the top border.
func (m *Model) viewTwoColumn(contentWidth int, margin, footer string, logoLines []string, logoHeight, availableHeight int, reveal float64) string {
	railW := railWidthFor(contentWidth, m.railWidthRatio)
	campW := contentWidth - railW - twoColGap
	m.leftColWidth = railW

	viewHeight := availableHeight - logoHeight - tavernTopPad
	if viewHeight < 1 {
		viewHeight = 1
	}
	m.lastViewHeight = viewHeight

	railRows := m.railColumnRows()
	campRows := m.campaignColumnRows()

	railIdx, campIdx := -1, -1
	if m.railFocus {
		railIdx = m.resolveColumnCursor(railRows)
	} else {
		campIdx = m.resolveColumnCursor(campRows)
	}

	m.hintSpans = map[int][]hintSpan{}
	m.codeSpans = map[int][]codeSpan{}

	railLines, railMap := m.renderRail(railRows, railIdx, railW, viewHeight, reveal)
	campContentX := m.leftMargin + railW + twoColGap + 2
	campLines, campMap := m.renderCampaigns(campRows, campIdx, campW, viewHeight, campContentX, reveal)

	m.railLineRow = railMap
	m.campLineRow = campMap
	m.railColX = m.leftMargin + 2
	m.campColX = campContentX

	columnGap := m.columnGap()
	var columnLines []string
	for i := 0; i < viewHeight; i++ {
		columnLines = append(columnLines, padOr(railLines, i, railW)+columnGap+padOr(campLines, i, campW))
	}

	m.rowsScreenTop = tavernTopPad + logoHeight
	// Record the cursor's on-screen cell in whichever column holds it, so
	// overlay effects (completion burst) fire at the right place in the Tavern
	// too — screen-space, same as the single-column path.
	m.cursorScreenY = -1
	if m.railFocus && railIdx >= 0 {
		if off := firstOffset(railMap, railIdx); off >= 0 {
			m.cursorScreenY = m.rowsScreenTop + off
			m.cursorScreenX = m.railColX // the cursor "›" marker column
		}
	} else if !m.railFocus && campIdx >= 0 {
		if off := firstOffset(campMap, campIdx); off >= 0 {
			m.cursorScreenY = m.rowsScreenTop + off
			m.cursorScreenX = m.campColX
		}
	}
	m.modeToggleRow = tavernTopPad
	m.chipLineRow = tavernTopPad + len(logoLines) + 1

	clip := lipgloss.NewStyle().MaxWidth(m.width)
	var b strings.Builder
	for i := 0; i < tavernTopPad; i++ {
		b.WriteString("\n") // breathing room above the TAVERN / WILDS header
	}
	for _, line := range logoLines {
		b.WriteString(margin + line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(clip.Render(m.renderFilterLine(contentWidth, margin)) + "\n")
	b.WriteString("\n")
	for _, line := range columnLines {
		b.WriteString(clip.Render(margin+line) + "\n")
	}
	m.cursorMoved = false // consumed for this frame; the wheel scrolls freely next
	return strings.TrimRight(b.String(), "\n") + "\n" + footer
}

func padOr(lines []string, i, w int) string {
	if i < len(lines) {
		return lines[i]
	}
	return strings.Repeat(" ", w)
}

// renderRail lays out the three rail boxes top-to-bottom, shrinking the tallest
// (which then scroll) until they all fit the rail height. Returns exactly
// `height` lines and a line→row map.
func (m *Model) renderRail(rows []ui.Row, activeIdx, colW, height int, reveal float64) ([]string, []int) {
	spans := sectionSpans(rows)
	innerW := colW - 4

	// Wrap each box's content ONCE (used for both sizing and rendering).
	contents := make([][]string, len(spans))
	contentRows := make([][]int, len(spans))
	natural := make([]int, len(spans))
	for i, sp := range spans {
		contents[i], contentRows[i] = m.boxContent(rows[sp[0]].Section, rows, sp[0], sp[1], sp[2], activeIdx, innerW, m.leftMargin+2, false)
		natural[i] = boxNaturalHeight(rows[sp[0]].Collapsed, len(contents[i]), true)
	}

	const minH = 4
	// Stacked boxes' borders sit directly adjacent (no dedicated blank row) —
	// matching the column gap's own tight, single-unit footprint (a text row
	// reads taller than a column reads wide, so a whole extra blank ROW looks
	// disproportionately bigger than the 1-column gap; touching borders plus
	// an on-hover recolor of the seam is the closer visual match).
	var heights []int
	if len(spans) == 3 {
		// A collapsed box always gets a fixed height (title bar only); the
		// REST of the rail's height is split among the non-collapsed boxes by
		// their own relative ratios — so collapsing one section visibly hands
		// its freed room to the others instead of leaving it as dead unused
		// space below the stack. m.railBoxRatios itself is never touched
		// here (only read), so un-collapsing later restores that box's exact
		// prior share rather than leaving it squished to its floor.
		var collapsed [3]bool
		collapsedCount := 0
		for i, sp := range spans {
			collapsed[i] = rows[sp[0]].Collapsed
			if collapsed[i] {
				collapsedCount++
			}
		}
		open := distributeOpenHeights(m.railBoxRatios, collapsed, height-2*collapsedCount, minH)
		heights = make([]int, 3)
		for i := range heights {
			if collapsed[i] {
				heights[i] = 2
			} else {
				heights[i] = open[i]
			}
		}
	} else {
		heights = append([]int(nil), natural...)
	}
	total := 0
	for _, h := range heights {
		total += h
	}
	// Safety net for a pathologically short terminal where even minH per open
	// box doesn't fit — shrinks the tallest until the stack fits. Not
	// expected to fire in practice since distributeOpenHeights already sizes
	// to fit exactly.
	for total > height {
		tallest := -1
		for i := range heights {
			if heights[i] > minH && (tallest < 0 || heights[i] > heights[tallest]) {
				tallest = i
			}
		}
		if tallest < 0 {
			break
		}
		heights[tallest]--
		total--
	}

	if len(spans) == 3 {
		m.lastRailHeights = [3]int{heights[0], heights[1], heights[2]}
	}

	var lines []string
	var lineMap []int
	m.railLineSection = m.railLineSection[:0]
	for i, sp := range spans {
		sec := rows[sp[0]].Section
		box, boxMap := m.assembleBox(sec, rows[sp[0]], sp[0], activeIdx, contents[i], contentRows[i], colW, heights[i], true, reveal)
		// This box's bottom border IS the divider below it — recolor just
		// that one line to accent while it's hovered/dragged, the same
		// "line indicates you can grab it" cue the column gap uses.
		if len(spans) == 3 && i < 2 {
			target := resizeRail0
			if i == 1 {
				target = resizeRail1
			}
			if m.resizeDrag.target == target || m.resizeHover == target {
				box[len(box)-1] = highlightBottomBorder(sec, colW)
			}
		}
		lines = append(lines, box...)
		lineMap = append(lineMap, boxMap...)
		for range box {
			m.railLineSection = append(m.railLineSection, sec)
		}
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", colW))
		lineMap = append(lineMap, -1)
		m.railLineSection = append(m.railLineSection, "")
	}
	return lines[:height], lineMap[:height]
}

// highlightBottomBorder redraws a box's bottom border row in accent color —
// used to indicate the divider directly below (a click-drag target) is
// hovered or being dragged, without reserving any extra row for it.
func highlightBottomBorder(section string, colW int) string {
	b := sectionBorder(section)
	style := lipgloss.NewStyle().Foreground(ui.ColorAccent)
	return style.Render(b.BottomLeft + strings.Repeat(b.Bottom, colW-2) + b.BottomRight)
}

// columnGap is the two-column gap's fill, the same on every line: a bright
// accent vertical line, centered in the gap (equal blank padding either
// side), while the column divider is hovered or being dragged; otherwise
// plain blank.
func (m *Model) columnGap() string {
	if m.resizeDrag.target != resizeColumns && m.resizeHover != resizeColumns {
		return strings.Repeat(" ", twoColGap)
	}
	left := (twoColGap - 1) / 2
	right := twoColGap - 1 - left
	return strings.Repeat(" ", left) + lipgloss.NewStyle().Foreground(ui.ColorAccent).Render("│") + strings.Repeat(" ", right)
}

// distributeOpenHeights splits `total` rows among the non-collapsed boxes by
// their relative railBoxRatios weight (renormalized among just themselves —
// a collapsed box's ratio doesn't count against the split), each floored at
// minH. Falls back to an even split among the open boxes if their ratios are
// degenerate (e.g. a hand-edited config.toml summing to 0). Collapsed slots
// are left at the zero value; the caller fills those in separately.
func distributeOpenHeights(ratios [3]float64, collapsed [3]bool, total, minH int) [3]int {
	var sum float64
	openCount := 0
	for i, c := range collapsed {
		if !c {
			sum += ratios[i]
			openCount++
		}
	}
	var out [3]int
	if openCount == 0 {
		return out
	}
	if sum <= 0 {
		sum = float64(openCount)
		for i, c := range collapsed {
			if !c {
				ratios[i] = 1
			}
		}
	}
	for i, c := range collapsed {
		if c {
			continue
		}
		h := int(float64(total)*ratios[i]/sum + 0.5)
		if h < minH {
			h = minH
		}
		out[i] = h
	}
	return out
}

// renderCampaigns renders the campaigns column as one scroll-box filling the
// height, content scrolled to keep the cursor visible while the frame stays.
func (m *Model) renderCampaigns(rows []ui.Row, activeIdx, colW, height, contentX int, reveal float64) ([]string, []int) {
	if len(rows) == 0 {
		rows = []ui.Row{{Kind: ui.RowLabel, Label: "Campaigns"}}
	}
	innerW := colW - 4
	content, contentRows := m.boxContent("campaigns", rows, 0, 1, len(rows), activeIdx, innerW, contentX, true)
	return m.assembleBox("campaigns", rows[0], 0, activeIdx, content, contentRows, colW, height, false, reveal)
}

func (m *Model) resolveColumnCursor(rows []ui.Row) int {
	idx := findRowIndex(rows, m.cursor)
	if idx < 0 && len(rows) > 0 {
		if r, ok := nearestSelectableRow(rows, 0); ok {
			m.setCursor(r)
			idx = findRowIndex(rows, m.cursor)
		}
	}
	return idx
}

func (m *Model) switchColumn(toRail bool) {
	if !m.twoColumn() || toRail == m.railFocus {
		return
	}
	m.commitEdit()
	m.rememberColumnCursor()
	m.railFocus = toRail
	m.restoreColumnCursor()
}

func (m *Model) rememberColumnCursor() {
	if m.railFocus {
		m.railCursor = m.cursor
	} else {
		m.campCursor = m.cursor
	}
}

func (m *Model) restoreColumnCursor() {
	var rows []ui.Row
	var want cursorTarget
	if m.railFocus {
		rows, want = m.railColumnRows(), m.railCursor
	} else {
		rows, want = m.campaignColumnRows(), m.campCursor
	}
	if idx := findRowIndex(rows, want); idx >= 0 {
		m.setCursor(rows[idx])
		return
	}
	if r, ok := nearestSelectableRow(rows, 0); ok {
		m.setCursor(r)
	}
}

func (m *Model) jumpToSection(section string) {
	toRail := section == "inbox" || section == "runes" || section == "someday"
	if m.twoColumn() && toRail != m.railFocus {
		m.commitEdit()
		m.rememberColumnCursor()
		m.railFocus = toRail
	}
	rows := m.visibleRows()
	target := ui.Row{Kind: ui.RowSection, Section: section}
	if section == "campaigns" {
		target = ui.Row{Kind: ui.RowLabel, Label: "Campaigns"}
	}
	if idx := findRowIndex(rows, targetFromRow(target)); idx >= 0 {
		m.commitEdit()
		m.setCursor(rows[idx])
	}
}

// toggleSectionCollapse collapses/expands a rail section (inbox/runes/someday)
// and lands the cursor on its header so it stays valid even when the row it
// was on gets hidden. renderRail's distributeOpenHeights handles reflowing the
// freed/reclaimed height across the remaining open boxes.
func (m *Model) toggleSectionCollapse(section string) {
	m.jumpToSection(section)
	m.collapsedSections[section] = !m.collapsedSections[section]
	m.saveLayoutConfig()
	m.invalidateRender()
}

func (m *Model) caretAtStart() bool {
	return m.editor == nil || m.editor.Position() == 0
}

func (m *Model) caretAtEnd() bool {
	return m.editor == nil || m.editor.Position() >= len([]rune(m.editor.Value()))
}

// sectionAtPoint is the Tavern section the pointer is over: "campaigns" for
// the right column, or the rail section (from railLineSection) for the left.
func (m *Model) sectionAtPoint(msg tea.Mouse) string {
	if msg.X >= m.leftMargin+m.leftColWidth+twoColGap {
		return "campaigns"
	}
	off := msg.Y - m.rowsScreenTop
	if off >= 0 && off < len(m.railLineSection) {
		return m.railLineSection[off]
	}
	return ""
}

// cursorSection is the section the cursor currently lives in.
// wheelSection scrolls a section's own scroll view by delta (clamped), leaving
// the cursor where it is — like a real viewport. The cursor is re-centered only
// when the user next moves it by keyboard (see assembleBox / cursorMoved).
func (m *Model) wheelSection(section string, delta int) {
	if section == "" {
		return
	}
	next := m.sectionScroll[section] + delta
	if next < 0 {
		next = 0
	}
	if max := m.sectionMaxScroll[section]; next > max {
		next = max
	}
	m.sectionScroll[section] = next
}

// handleTwoColumnClick routes a left-press to the column it landed in and the
// row the line→row map resolves it to.
func (m *Model) handleTwoColumnClick(msg tea.Mouse) tea.Cmd {
	campStartX := m.leftMargin + m.leftColWidth + twoColGap
	if msg.X >= campStartX {
		if m.railFocus {
			m.railCursor = m.cursor
			m.railFocus = false
		}
		off := msg.Y - m.rowsScreenTop
		return m.clickRowAt(m.campaignColumnRows(), lineFrom(m.campLineRow, off), msg, m.campColX)
	}
	return m.clickRailAt(msg.Y-m.rowsScreenTop, msg)
}

// clickRailAt handles a click at rail line `off`: focuses the rail, moves the
// cursor to that row, and acts by kind.
func (m *Model) clickRailAt(off int, msg tea.Mouse) tea.Cmd {
	rows := m.railColumnRows()
	idx := lineFrom(m.railLineRow, off)
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	if !row.Selectable() {
		return nil
	}
	if !m.railFocus {
		m.campCursor = m.cursor
	}
	m.railFocus = true
	m.commitEdit()
	m.setCursor(row)

	if cmd, ok := m.commonRowClick(row); ok {
		return cmd
	}

	relX := msg.X - m.railColX
	switch row.Kind {
	case ui.RowSection:
		// Consistent across all sections: chevron collapses, name opens the
		// section's focused view.
		if relX <= 1 {
			m.toggleReveal()
			return nil
		}
		return m.handleReveal()
	case ui.RowQuest:
		m.beginTextSelection(m.editor, relX-titleOffset(row, 0))
		return nil
	}
	return nil
}
