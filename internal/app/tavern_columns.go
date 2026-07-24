package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// twoColGap is the number of blank columns between the left rail and the right
// campaigns column.
const twoColGap = 4

// ellipsis marks a section box that has more content scrolled out of view.
const ellipsis = "···"

// tavernTopPad is the blank rows above the TAVERN / WILDS header.
const tavernTopPad = 2

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

// railWidthFor sizes the left rail; the wider campaigns list takes the rest.
func railWidthFor(contentWidth int) int {
	rail := contentWidth * 34 / 100
	rail = clampInt(rail, 40, 64)
	if camp := contentWidth - rail - twoColGap; camp < 44 {
		rail = contentWidth - twoColGap - 44
	}
	if rail < 24 {
		rail = 24
	}
	return rail
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
func sectionColor(section string) lipgloss.TerminalColor {
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
		return "▤"
	case "runes":
		return "◈"
	case "someday":
		return "▦"
	case "campaigns":
		return "⚑"
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
	if e := m.boxCache[section]; e != nil && e.uiVersion == m.uiVersion && e.innerW == innerW && e.cursor == m.cursor && e.collapsed == collapsed {
		for k, v := range e.hint {
			m.hintSpans[k] = v
		}
		for k, v := range e.code {
			m.codeSpans[k] = v
		}
		return e.content, e.rows
	}

	// Miss: render into fresh span maps so we can cache exactly this section's
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

	if header.Collapsed {
		m.sectionMaxScroll[section] = 0
		box := drawBox(m.boxTitle(section, header, active), sectionMotif(section), nil, colW, sectionBorder(section), frameStyle(section, active))
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

	box := drawBox(m.boxTitle(section, header, active), sectionMotif(section), interior, colW, sectionBorder(section), frameStyle(section, active))

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
	railW := railWidthFor(contentWidth)
	campW := contentWidth - railW - twoColGap
	m.leftColWidth = railW

	viewHeight := availableHeight - logoHeight - tavernTopPad
	if viewHeight < 1 {
		viewHeight = 1
	}

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

	gap := strings.Repeat(" ", twoColGap)
	var columnLines []string
	for i := 0; i < viewHeight; i++ {
		columnLines = append(columnLines, padOr(railLines, i, railW)+gap+padOr(campLines, i, campW))
	}

	m.rowsScreenTop = tavernTopPad + logoHeight
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
	heights := make([]int, len(spans))
	total := 0
	for i, sp := range spans {
		contents[i], contentRows[i] = m.boxContent(rows[sp[0]].Section, rows, sp[0], sp[1], sp[2], activeIdx, innerW, m.leftMargin+2, false)
		heights[i] = boxNaturalHeight(rows[sp[0]].Collapsed, len(contents[i]), true)
		total += heights[i]
	}
	// Shrink the tallest box (which then scrolls) until the stack fits.
	const minH = 4
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

	var lines []string
	var lineMap []int
	m.railLineSection = m.railLineSection[:0]
	for i, sp := range spans {
		sec := rows[sp[0]].Section
		box, boxMap := m.assembleBox(sec, rows[sp[0]], sp[0], activeIdx, contents[i], contentRows[i], colW, heights[i], true, reveal)
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

func (m *Model) caretAtStart() bool {
	return m.editor == nil || m.editor.Position() == 0
}

func (m *Model) caretAtEnd() bool {
	return m.editor == nil || m.editor.Position() >= len([]rune(m.editor.Value()))
}

// sectionAtPoint is the Tavern section the pointer is over: "campaigns" for
// the right column, or the rail section (from railLineSection) for the left.
func (m *Model) sectionAtPoint(msg tea.MouseMsg) string {
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
func (m *Model) handleTwoColumnClick(msg tea.MouseMsg) tea.Cmd {
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
func (m *Model) clickRailAt(off int, msg tea.MouseMsg) tea.Cmd {
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

	relX := msg.X - m.railColX
	switch row.Kind {
	case ui.RowSection:
		// Only the Questboard has a useful focused page; a click on its title
		// body opens it, on the caret toggles. Runes/Vault titles just toggle.
		if row.Section == "inbox" && relX > 1 {
			return m.handleReveal()
		}
		m.toggleReveal()
		return nil
	case ui.RowQuest:
		m.beginTextSelection(m.editor, relX-titleOffset(row, 0))
		return nil
	case ui.RowRuneQuest:
		m.toggleReveal() // collapse/expand the quest's rune group
		return nil
	case ui.RowRune:
		return openURL(ldFlagURL(m.ldProject, m.ldEnv, row.RuneKey))
	case ui.RowNewQuest, ui.RowNewRune:
		return m.handleEnter()
	}
	return nil
}
