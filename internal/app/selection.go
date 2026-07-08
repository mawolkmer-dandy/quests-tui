package app

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// noSelection is the Model.selAnchor sentinel for "nothing selected".
// bubbles/textinput has no selection concept of its own, so this app
// tracks one shared anchor rune-index alongside whichever single
// textinput.Model is currently focused (the row editor, a body line, or
// the search box — only one is ever live at a time in this outline). The
// other end of the range is always that textinput's own cursor position.
const noSelection = -1

// clearSelection drops any active selection — call whenever the focused
// editor changes to a different field (a new row, a new body line, a
// freshly opened modal) so a stale range from one field can't appear to
// apply to another.
func (m *Model) clearSelection() {
	m.selAnchor = noSelection
	m.selAnchorLine = noSelection
}

// selectionBounds returns the ordered [start,end) rune range currently
// selected in ti, or ok=false if there's none (or it's collapsed to a
// single point, e.g. right after Shift+Left brought the cursor back to
// the anchor). The anchor is clamped against ti's own current length
// before use — it's a bare rune index shared across whichever editor is
// focused, so if a caller ever forgets to clearSelection when switching to
// a different (e.g. shorter) field, this stops it from indexing out of
// that field's bounds rather than just showing a wrong-but-safe range.
func (m *Model) selectionBounds(ti *textinput.Model) (start, end int, ok bool) {
	if m.selAnchor == noSelection {
		return 0, 0, false
	}
	anchor := clampInt(m.selAnchor, 0, len([]rune(ti.Value())))
	cur := ti.Position()
	if anchor == cur {
		return 0, 0, false
	}
	if anchor < cur {
		return anchor, cur, true
	}
	return cur, anchor, true
}

// applySelectionKey intercepts Shift+Left/Right/Home/End to extend a text
// selection — bubbles/textinput doesn't understand Shift at all, so left
// unhandled these would silently do nothing. The selection is copied to
// the system clipboard as it grows, so selecting text is all it takes to
// grab it (e.g. a URL) — no separate copy key needed.
//
// Any other key drops the current selection. If it's Backspace or Delete,
// removing the selection is that keypress's entire effect (like a normal
// text editor) — the caller must not also forward it to ti.Update, or
// textinput's own Backspace/Delete handling would remove a second
// character on top of it. A plain typed rune instead replaces the
// selection: it's deleted here, but the caller should still forward the
// rune to ti.Update so it's inserted at the now-collapsed cursor.
//
// Returns (true, cmd) if msg was fully handled here and must not be
// forwarded to ti.Update; cmd shows the "copied to clipboard" toast when a
// selection just grew.
func (m *Model) applySelectionKey(ti *textinput.Model, msg tea.KeyMsg) (bool, tea.Cmd) {
	runes := []rune(ti.Value())

	switch msg.String() {
	case "shift+left":
		return true, m.extendSelection(ti, clampInt(ti.Position()-1, 0, len(runes)))
	case "shift+right":
		return true, m.extendSelection(ti, clampInt(ti.Position()+1, 0, len(runes)))
	case "shift+home":
		return true, m.extendSelection(ti, 0)
	case "shift+end":
		return true, m.extendSelection(ti, len(runes))
	}

	start, end, hasSelection := m.selectionBounds(ti)
	// Any non-shift key drops the anchor even when the selection is still
	// collapsed (e.g. right after a plain click armed one) — otherwise a
	// later Shift+End would extend from the stale click position instead of
	// from wherever the cursor has since moved.
	m.clearSelection()
	if !hasSelection {
		return false, nil
	}
	if !isTypingKey(msg) {
		return false, nil // plain navigation, etc. — just drop the highlight
	}
	ti.SetValue(string(runes[:start]) + string(runes[end:]))
	ti.SetCursor(start)
	// Backspace/Delete are fully consumed by removing the selection; a
	// typed rune or Enter still proceeds (inserting / splitting at the
	// now-collapsed cursor), so typing over a selection replaces it.
	return msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete, nil
}

func (m *Model) extendSelection(ti *textinput.Model, newPos int) tea.Cmd {
	if m.selAnchor == noSelection {
		m.selAnchor = ti.Position()
	}
	ti.SetCursor(newPos)
	return m.copySelection(ti)
}

// copySelection writes the current selection (if any) to the system
// clipboard — best-effort, silently ignored if the platform has no
// clipboard utility available — and returns a command that briefly shows
// the "copied to clipboard" indicator.
func (m *Model) copySelection(ti *textinput.Model) tea.Cmd {
	start, end, ok := m.selectionBounds(ti)
	if !ok {
		return nil
	}
	runes := []rune(ti.Value())
	_ = clipboard.WriteAll(string(runes[start:end]))
	return m.showClipboardToast()
}

func isTypingKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes, tea.KeySpace, tea.KeyBackspace, tea.KeyDelete, tea.KeyEnter:
		return true
	}
	return false
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// renderEditableText renders ti's value with the active selection
// highlighted and a block cursor, replacing ti.View() — bubbles/textinput
// has no concept of a selection range to draw.
func (m *Model) renderEditableText(ti *textinput.Model) string {
	return m.renderEditableStyled(ti, lipgloss.NewStyle())
}

// renderEditableStyled is renderEditableText with a base text style applied
// to every rune, so an editable title keeps its look (bold, or done's faint
// strikethrough) while it's the cursor row rather than dropping to plain.
func (m *Model) renderEditableStyled(ti *textinput.Model, base lipgloss.Style) string {
	start, end, hasSelection := m.selectionBounds(ti)
	runes := []rune(ti.Value())
	cursor := ti.Position()

	if len(runes) == 0 {
		block := lipgloss.NewStyle().Reverse(true).Render(" ")
		if ti.Placeholder != "" {
			return block + ui.StyleMuted.Render(ti.Placeholder)
		}
		return block
	}

	var b strings.Builder
	for i, r := range runes {
		style := base
		if hasSelection && i >= start && i < end {
			style = style.Background(ui.ColorSelected)
		}
		if i == cursor {
			style = style.Reverse(true)
		}
		b.WriteString(style.Render(string(r)))
	}
	if cursor >= len(runes) {
		b.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
	}
	return b.String()
}

// cursorTitleStyle is the base style for the row currently being edited: a
// done quest keeps its faint strikethrough, everything else stays bold —
// matching how the same row renders when it's NOT the cursor.
func (m *Model) cursorTitleStyle(row ui.Row) lipgloss.Style {
	if row.Kind == ui.RowQuest {
		if q := m.findQuest(row.QuestID); q != nil && q.Status == model.StatusDone {
			return ui.StyleDone
		}
	}
	return ui.StyleTitle
}

// renderClipboardToast is the "● copied to clipboard" indicator shown
// briefly (see showClipboardToast) in place of the footer/header pointer.
func renderClipboardToast() string {
	bullet := lipgloss.NewStyle().Bold(true).Foreground(ui.ColorHeading).Render("●")
	return bullet + " " + ui.StyleMuted.Render("copied to clipboard")
}

// --- multiline selection (focus-view bodies only) -----------------------
//
// A selection that spans body lines is anchored at (selAnchorLine,
// selAnchor) with its moving end at (BodyCursor, BodyEditor.Position()).
// It's COPY-ONLY: once a selection crosses lines, typing/Backspace just
// drop it rather than deleting across lines — the point is grabbing text
// (a URL, a paragraph), not multi-line editing, and line merge/split
// semantics aren't worth the edge cases.

// multilineSelActive reports whether the current selection spans more than
// one body line of an open focus view.
func (m *Model) multilineSelActive() bool {
	mod := m.modal
	return mod != nil && isFocusModal(mod.Kind) &&
		m.selAnchor != noSelection && m.selAnchorLine != noSelection &&
		m.selAnchorLine != mod.BodyCursor
}

// applyBodySelectionKey is applySelectionKey for a focus view's body
// outline — same Shift+Left/Right/Home/End handling, plus Shift+Up/Down to
// grow the selection across lines.
func (m *Model) applyBodySelectionKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	mod := m.modal
	runes := []rune(mod.BodyEditor.Value())

	switch msg.String() {
	case "shift+up":
		return true, m.extendBodySelectionLine(-1)
	case "shift+down":
		return true, m.extendBodySelectionLine(1)
	case "shift+left":
		return true, m.extendBodySelectionCol(clampInt(mod.BodyEditor.Position()-1, 0, len(runes)))
	case "shift+right":
		return true, m.extendBodySelectionCol(clampInt(mod.BodyEditor.Position()+1, 0, len(runes)))
	case "shift+home":
		return true, m.extendBodySelectionCol(0)
	case "shift+end":
		return true, m.extendBodySelectionCol(len(runes))
	}

	if m.multilineSelActive() {
		switch {
		case msg.Type == tea.KeyBackspace, msg.Type == tea.KeyDelete:
			// Delete the whole cross-line selection, not a single char.
			m.deleteBodySelection()
			return true, nil
		case isTypingKey(msg):
			// Replace the selection: collapse it, then let the key insert.
			m.deleteBodySelection()
			return false, nil
		default:
			// Any other key (navigation etc.) just drops the selection.
			m.clearSelection()
			return false, nil
		}
	}
	return m.applySelectionKey(&mod.BodyEditor, msg)
}

func (m *Model) extendBodySelectionCol(newPos int) tea.Cmd {
	mod := m.modal
	if m.selAnchor == noSelection {
		m.selAnchor = mod.BodyEditor.Position()
		m.selAnchorLine = mod.BodyCursor
	}
	mod.BodyEditor.SetCursor(newPos)
	return m.copyBodySelection()
}

// extendBodySelectionLine grows the selection one line up/down, moving the
// editing focus to that line while keeping the anchor where the selection
// started. At the first/last line it extends to the line's start/end
// instead, so repeated presses still make progress.
func (m *Model) extendBodySelectionLine(delta int) tea.Cmd {
	mod := m.modal
	body := m.currentBody()
	if body == nil {
		return nil
	}
	if m.selAnchor == noSelection {
		m.selAnchor = mod.BodyEditor.Position()
		m.selAnchorLine = mod.BodyCursor
	}
	m.commitBodyLine()

	newIdx := clampInt(mod.BodyCursor+delta, 0, len(*body)-1)
	if newIdx == mod.BodyCursor {
		if delta < 0 {
			mod.BodyEditor.SetCursor(0)
		} else {
			mod.BodyEditor.SetCursor(len([]rune(mod.BodyEditor.Value())))
		}
		return m.copyBodySelection()
	}

	col := mod.BodyEditor.Position()
	mod.BodyCursor = newIdx
	ed := bodyLineEditor((*body)[newIdx].Text) // not newBodyEditor — the anchor must survive
	ed.SetCursor(col)
	mod.BodyEditor = ed
	return m.copyBodySelection()
}

// bodySelEndpoints returns the ordered (startLine, startCol, endLine,
// endCol) of the active cross-line selection.
func (m *Model) bodySelEndpoints() (sL, sC, eL, eC int) {
	mod := m.modal
	sL, sC = m.selAnchorLine, m.selAnchor
	eL, eC = mod.BodyCursor, mod.BodyEditor.Position()
	if sL > eL || (sL == eL && sC > eC) {
		sL, sC, eL, eC = eL, eC, sL, sC
	}
	return sL, sC, eL, eC
}

// bodyLineSelRange returns the selected [lo,hi) raw-rune range on body
// line i under the active cross-line selection, or ok=false if line i is
// outside it (or no cross-line selection is active).
func (m *Model) bodyLineSelRange(i, lineLen int) (lo, hi int, ok bool) {
	if !m.multilineSelActive() {
		return 0, 0, false
	}
	sL, sC, eL, eC := m.bodySelEndpoints()
	if i < sL || i > eL {
		return 0, 0, false
	}
	lo, hi = 0, lineLen
	if i == sL {
		lo = clampInt(sC, 0, lineLen)
	}
	if i == eL {
		hi = clampInt(eC, 0, lineLen)
	}
	return lo, hi, true
}

// deleteBodySelection removes the active cross-line selection: line sL keeps
// its text up to sC, line eL's text from eC is appended onto it, the lines
// between collapse away, and the caret lands at the join on line sL.
func (m *Model) deleteBodySelection() {
	mod := m.modal
	body := m.currentBody()
	if body == nil || !m.multilineSelActive() {
		return
	}
	sL, sC, eL, eC := m.bodySelEndpoints()
	sL = clampInt(sL, 0, len(*body)-1)
	eL = clampInt(eL, 0, len(*body)-1)

	startText, endText := (*body)[sL].Text, (*body)[eL].Text
	if sL == mod.BodyCursor {
		startText = mod.BodyEditor.Value()
	}
	if eL == mod.BodyCursor {
		endText = mod.BodyEditor.Value()
	}
	sr, er := []rune(startText), []rune(endText)
	sC = clampInt(sC, 0, len(sr))
	eC = clampInt(eC, 0, len(er))

	(*body)[sL].Text = string(sr[:sC]) + string(er[eC:])
	*body = append((*body)[:sL+1], (*body)[eL+1:]...)
	m.seedBodyEditor(sL, sC) // also clears the selection (newBodyEditor)
	m.touchBodyOwner()
}

// copyBodySelection copies the current focus-view selection — single-line
// selections go through the shared copySelection; cross-line ones join the
// covered lines' raw text with newlines.
func (m *Model) copyBodySelection() tea.Cmd {
	mod := m.modal
	body := m.currentBody()
	if body == nil || m.selAnchor == noSelection {
		return nil
	}
	if !m.multilineSelActive() {
		return m.copySelection(&mod.BodyEditor)
	}

	sL, sC, eL, eC := m.bodySelEndpoints()
	var parts []string
	for i := sL; i <= eL && i < len(*body); i++ {
		if i < 0 {
			continue
		}
		text := (*body)[i].Text
		if i == mod.BodyCursor {
			text = mod.BodyEditor.Value() // live, possibly uncommitted edits
		}
		r := []rune(text)
		lo, hi := 0, len(r)
		if i == sL {
			lo = clampInt(sC, 0, len(r))
		}
		if i == eL {
			hi = clampInt(eC, 0, len(r))
		}
		if lo > hi {
			lo = hi
		}
		parts = append(parts, string(r[lo:hi]))
	}
	_ = clipboard.WriteAll(strings.Join(parts, "\n"))
	return m.showClipboardToast()
}

// wrapSegments greedily word-wraps runes to width columns, returning the
// [start,end) rune range of each wrapped row — a word longer than the
// whole width hard-breaks. Always returns at least one segment.
func wrapSegments(runes []rune, width int) [][2]int {
	if width < 1 {
		width = 1
	}
	var segs [][2]int
	start := 0
	for start < len(runes) {
		if len(runes)-start <= width {
			segs = append(segs, [2]int{start, len(runes)})
			break
		}
		br := -1
		for i := start + width; i > start; i-- {
			if runes[i-1] == ' ' {
				br = i
				break
			}
		}
		if br <= start {
			br = start + width
		}
		segs = append(segs, [2]int{start, br})
		start = br
	}
	if len(segs) == 0 {
		segs = [][2]int{{0, 0}}
	}
	return segs
}

// renderBodyLineWrapped renders body line i as one or more screen rows,
// soft-wrapping at width; continuation rows are indented to align under
// the first row's text. It handles the edited line (live editor value,
// cursor block, selection highlight) and non-edited lines (classified
// display text, its slice of a cross-line selection) alike, and appends
// each emitted row's (line index, raw rune offset) to the focus row map
// used by handleFocusMouse for hit-testing.
// renderBodyLineWrapped returns the line's screen rows and, if it's the
// edited (caret) line, the index within those rows where the caret sits
// (-1 otherwise) — the caller uses that to keep the caret in view when the
// focus content scrolls.
func (m *Model) renderBodyLineWrapped(i int, l model.BodyLine, editing bool, width int) (rows []string, caretRow int) {
	mod := m.modal
	caretRow = -1

	kind, display := model.ClassifyBodyLine(l.Text)

	// Nesting: 2 columns per level, added between the cursor mark and the
	// line's lead glyph so deeper items sit further right and text stays
	// aligned under itself across wrapped rows.
	indent := strings.Repeat(" ", 2*l.Indent)
	effWidth := width - 2*l.Indent
	if effWidth < 8 {
		effWidth = 8
	}

	lead := "  "
	if kind == model.BodyObjective {
		check := ui.GlyphQuestOpen
		if l.Done {
			check = ui.GlyphQuestDone
		}
		lead = ui.StyleMuted.Render(check) + " "
	}

	addRow := func(rawOffset int) {
		m.focusRowLine = append(m.focusRowLine, i)
		m.focusRowOffset = append(m.focusRowOffset, rawOffset)
	}
	// head is [cursor mark 2][indent][lead 2] on the first row, and a plain
	// run of spaces of the same width on continuation rows. The "›" marks
	// the caret's visual row so it follows wrapped-line navigation; the
	// checkbox stays on the line's first row.
	head := func(rowIdx int, caret bool) string {
		mark := "  "
		if caret {
			mark = ui.StyleCursor.Render(ui.GlyphCursor)
		}
		if rowIdx == 0 {
			return mark + indent + lead
		}
		return strings.Repeat(" ", 4+2*l.Indent)
	}

	if editing {
		raw := []rune(mod.BodyEditor.Value())
		cursor := mod.BodyEditor.Position()
		// Derive the glyph AND the text style from the LIVE editor value, so
		// the line keeps its heading/done look while being edited (and a "- "
		// typed onto a plain line grows its checkbox immediately).
		liveKind, _ := model.ClassifyBodyLine(string(raw))
		lead = "  "
		if liveKind == model.BodyObjective {
			g := ui.GlyphQuestOpen
			if l.Done {
				g = ui.GlyphQuestDone
			}
			lead = ui.StyleMuted.Render(g) + " "
		}
		base := lipgloss.NewStyle()
		switch {
		case liveKind == model.BodyHeading:
			base = ui.StyleHeading
		case liveKind == model.BodyObjective && l.Done:
			base = ui.StyleDone
		}
		selLo, selHi, hasSel := 0, 0, false
		if m.multilineSelActive() {
			selLo, selHi, hasSel = m.bodyLineSelRange(i, len(raw))
		} else {
			selLo, selHi, hasSel = m.selectionBounds(&mod.BodyEditor)
		}
		if len(raw) == 0 {
			addRow(0)
			return []string{head(0, true) + lipgloss.NewStyle().Reverse(true).Render(" ")}, 0
		}
		segs := wrapSegments(raw, effWidth)
		// The caret's visual row — boundary between two wrapped rows
		// resolves to the later one, matching currentVisualRow.
		cursorSeg := 0
		for si, seg := range segs {
			if cursor >= seg[0] && cursor <= seg[1] {
				cursorSeg = si
			}
		}
		for si, seg := range segs {
			var b strings.Builder
			for j := seg[0]; j < seg[1]; j++ {
				st := base
				if hasSel && j >= selLo && j < selHi {
					st = st.Background(ui.ColorSelected)
				}
				if j == cursor {
					st = st.Reverse(true)
				}
				b.WriteString(st.Render(string(raw[j])))
			}
			if si == len(segs)-1 && cursor >= len(raw) {
				b.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
			}
			rows = append(rows, head(si, si == cursorSeg)+b.String())
			addRow(seg[0])
		}
		return rows, cursorSeg
	}

	// TODO(integrations): render any EXTRA Jira/PR URL on a non-edited body
	// line shortened to its code (model.ShortenLinks), clickable. Deferred
	// because shortening `display` shifts every downstream rune offset used by
	// the cross-line selection range mapping (bodyLineSelRange/strip) and the
	// focusRowOffset mouse hit-test map — reconciling those against the
	// pre-shorten raw text needs its own offset-translation layer to avoid
	// breaking selection and caret placement. The captured code is already
	// surfaced above the body (see renderFocusContent / focusCodeLines), so
	// links are still visible and clickable; only inline shortening of extras
	// is pending.
	dr := []rune(display)
	strip := len([]rune(l.Text)) - len(dr)
	selLo, selHi, hasSel := m.bodyLineSelRange(i, len([]rune(l.Text)))
	if hasSel {
		selLo = clampInt(selLo-strip, 0, len(dr))
		selHi = clampInt(selHi-strip, 0, len(dr))
	}
	base := lipgloss.NewStyle()
	switch {
	case kind == model.BodyHeading:
		base = ui.StyleHeading
	case kind == model.BodyObjective && l.Done:
		base = ui.StyleDone
	}
	if len(dr) == 0 {
		addRow(strip)
		return []string{head(0, false)}, -1
	}
	segs := wrapSegments(dr, effWidth)
	for si, seg := range segs {
		var b strings.Builder
		for j := seg[0]; j < seg[1]; j++ {
			st := base
			if hasSel && j >= selLo && j < selHi {
				st = st.Background(ui.ColorSelected)
			}
			b.WriteString(st.Render(string(dr[j])))
		}
		rows = append(rows, head(si, false)+b.String())
		addRow(seg[0] + strip)
	}
	return rows, -1
}
