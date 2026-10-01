package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// noteIndent is the left pad before a campaign note's prose, aligning it under
// the title text. Wrapped continuation lines share it (no cursor mark).
const noteIndent = "    "

// noteHeadWidth is the columns before a note's prose: a 2-col cursor mark plus
// the indent. A shortened link's clickable span is offset by it.
const noteHeadWidth = 2 + len(noteIndent)

// noteLinkRun is one shortened-link run on a note's wrapped line: seg is which
// of the note's screen lines it sits on; [x0,x1) are its column extents from the
// pane column's left edge; url is the full address to copy/open.
type noteLinkRun struct {
	seg    int
	x0, x1 int
	url    string
}

// renderNoteScreenLines renders one campaign-note row to its wrapped screen
// lines: prose soft-wraps at the pane width (breaking on spaces), so a long note
// or a pasted link flows onto continuation lines instead of clipping. Shortened
// links render blue (like a quest body); the runs are returned so the pane can
// make them click-to-copy / double-click-to-open. The active line shows the live
// body editor with a block caret; an empty campaign shows an "add notes…" hint.
func (m *Model) renderNoteScreenLines(row ui.Row, width int) (lines []string, runs []noteLinkRun) {
	isCursor := m.cursor.matches(row)
	editing := isCursor && m.bodyOwnerKind == ownerCampaign && m.bodyOwnerID == row.ProjectID
	cursorMark := "  "
	if isCursor {
		cursorMark = ui.StyleCursor.Render(ui.GlyphCursor)
	}
	avail := width - len(noteIndent)
	if avail < 8 {
		avail = 8
	}

	// Resolve the line's text, its link map, its body-line index, and (when
	// active) the caret.
	text, caret, li := "", -1, -1
	var links map[string]string
	if p := m.findProject(row.ProjectID); p != nil {
		links = p.BodyLinks
		if editing {
			text, caret, li = m.bodyEditor.Value(), m.bodyEditor.Position(), m.bodyCursor
		} else if row.BodyLineID != "" {
			for i, l := range p.Body {
				if l.ID == row.BodyLineID {
					text, li = l.Text, i
					break
				}
			}
		}
	}

	// The "add notes…" placeholder: an empty, unselected campaign note.
	if text == "" && caret < 0 {
		return []string{cursorMark + noteIndent + ui.StyleMuted.Render("add notes…")}, nil
	}

	runes := []rune(text)
	linkURL := noteLinkURLs(links, runes)

	// Highlight a text selection the same way the quest body does — the caret's
	// own line via selectionBounds, or any line under a cross-line selection.
	selLo, selHi, hasSel := 0, 0, false
	if m.multilineSelActive() {
		selLo, selHi, hasSel = m.bodyLineSelRange(li, len(runes))
	} else if editing {
		selLo, selHi, hasSel = m.selectionBounds(&m.bodyEditor)
	}

	segs := wrapSegments(runes, avail)
	if len(segs) == 0 {
		segs = [][2]int{{0, 0}} // an empty active line still needs a row for its caret
	}
	for k, seg := range segs {
		mark := "  "
		if k == 0 {
			mark = cursorMark
		}
		content, segRuns := renderNoteSegment(runes, seg[0], seg[1], caret, linkURL, selLo, selHi, hasSel)
		lines = append(lines, mark+noteIndent+content)
		for _, r := range segRuns {
			runs = append(runs, noteLinkRun{seg: k, x0: noteHeadWidth + r.x0, x1: noteHeadWidth + r.x1, url: r.url})
		}
	}
	return lines, runs
}

// renderNoteSegment renders rune range [s,e) of a note line: shortened links in
// blue, other prose muted, selected runes on a highlight background, with a
// reverse-video block caret where the caret falls (a rune in range, or the very
// end of the line). Returns the rendered string plus each link run's column
// extents relative to the segment start.
func renderNoteSegment(runes []rune, s, e, caret int, linkURL []string, selLo, selHi int, hasSel bool) (string, []noteLinkRun) {
	var b strings.Builder
	var runs []noteLinkRun
	// StyleSide (blue) is resolved at ui.Init; bolded per-render so a link reads
	// the same as in a quest body. Building it here (not as a package var) is why
	// it picks up the initialized color.
	linkStyle := ui.StyleSide.Bold(true)
	for i := s; i < e; i++ {
		st := ui.StyleMuted
		if i < len(linkURL) && linkURL[i] != "" {
			st = linkStyle
			if i == s || linkURL[i-1] != linkURL[i] {
				runs = append(runs, noteLinkRun{x0: i - s, x1: i - s + 1, url: linkURL[i]})
			} else {
				runs[len(runs)-1].x1 = i - s + 1
			}
		}
		if hasSel && i >= selLo && i < selHi {
			st = st.Background(ui.ColorSelected)
		}
		if i == caret {
			st = st.Reverse(true)
		}
		b.WriteString(st.Render(string(runes[i])))
	}
	if caret == e && e == len(runes) {
		b.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
	}
	return b.String(), runs
}

// noteLinkURLs returns a per-rune slice the length of runes where entry j is the
// full URL when rune j is part of a shortened-link display (a key of links),
// else "". Lets the renderer color link runs and the pane route clicks on them.
func noteLinkURLs(links map[string]string, runes []rune) []string {
	urls := make([]string, len(runes))
	for short, url := range links {
		sr := []rune(short)
		n := len(sr)
		if n == 0 {
			continue
		}
		for i := 0; i+n <= len(runes); i++ {
			matched := true
			for k := 0; k < n; k++ {
				if runes[i+k] != sr[k] {
					matched = false
					break
				}
			}
			if matched {
				for k := 0; k < n; k++ {
					urls[i+k] = url
				}
			}
		}
	}
	return urls
}

// Campaign notes are Project.Body rendered inline at the top of a campaign's
// Tavern pane and edited with the same shared outline editor a quest's detail
// modal uses — reusing its navigation, split/merge, and copy-paste. Plain prose
// only: objectives, headings, and nesting (a quest-body thing) are left out.

// enterCampaignNotes points the shared body editor at the campaign-notes line
// under the cursor. The "add notes…" placeholder (BodyLineID == "") leaves the
// body empty; the first content key materializes a line (handleCampaignNoteEditKey).
func (m *Model) enterCampaignNotes(row ui.Row) {
	p := m.findProject(row.ProjectID)
	if p == nil {
		m.seedBody(ownerNone, "")
		return
	}
	m.bodyOwnerKind = ownerCampaign
	m.bodyOwnerID = p.ID
	m.clearSelection()

	idx := 0
	for i, l := range p.Body {
		if l.ID == row.BodyLineID {
			idx = i
			break
		}
	}
	m.bodyCursor = idx
	text := ""
	if idx < len(p.Body) {
		text = p.Body[idx].Text
	}
	m.bodyEditor = m.newBodyEditor(text)
}

// syncNoteCursor repoints the pane cursor at the RowBodyLine matching the body
// editor's current line — called after an edit changed the body (split, merge,
// reorder) so the cursor keeps tracking the line the caret is really on.
func (m *Model) syncNoteCursor() {
	body := m.currentBody()
	if body == nil || len(*body) == 0 {
		return
	}
	idx := clampInt(m.bodyCursor, 0, len(*body)-1)
	m.cursor = cursorTarget{kind: ui.RowBodyLine, projectID: m.bodyOwnerID, bodyLineID: (*body)[idx].ID}
	m.cursorMoved = true
}

// handleCampaignNoteEditKey handles the editing keys for a campaign's inline
// notes by reusing the shared body outline editor — split/merge, copy-paste,
// and typing. Navigation (Up/Down between lines, up into the title, down into
// the quest list) flows through the ordinary pane cursor instead, so this
// returns handled=false for anything that isn't an edit. Objectives and nesting
// aren't part of plain notes, so Ctrl+D is swallowed and Tab is left to navigate.
func (m *Model) handleCampaignNoteEditKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case msg.String() == "ctrl+d":
		return nil, true // no objectives in campaign notes — swallow, no-op
	case msg.Code == tea.KeyTab:
		return nil, false // Tab navigates in the pane; it never indents notes
	}

	p := m.findProject(m.cursor.projectID)
	if p == nil {
		return nil, false
	}
	m.bodyOwnerKind, m.bodyOwnerID = ownerCampaign, p.ID

	// Materialize the first line from the "add notes…" placeholder, but only on
	// a real content key — plain navigation off an empty placeholder is left to
	// the ordinary pane cursor.
	if len(p.Body) == 0 {
		if msg.Text == "" && msg.Code != tea.KeyEnter && msg.Code != tea.KeySpace {
			return nil, false
		}
		p.Body = []model.BodyLine{{ID: store.NewID(), Text: ""}}
		m.bodyCursor = 0
		m.bodyEditor = m.newBodyEditor("")
		m.save()
		m.syncNoteCursor()
	}

	if handled, cmd := m.applyBodySelectionKey(msg); handled {
		m.syncNoteCursor()
		return cmd, true
	}
	if cmd, handled := m.handleBodyOutlineKey(msg); handled {
		m.syncNoteCursor()
		return cmd, true
	}
	// Everything else that reaches here is ordinary in-line editing — caret
	// moves (←/→/Home/End), typing, Ctrl+A/E/K/U/W — forwarded to the body
	// editor. (Up/Down never reach here: they're pane navigation handled a
	// level up, where commitEdit is body-aware.)
	var cmd tea.Cmd
	m.bodyEditor, cmd = m.bodyEditor.Update(msg)
	m.syncNoteCursor()
	return cmd, true
}

// pasteIntoCampaignNotes inserts a bracketed paste into the active note (the
// pane editor is nil for notes, so app.go routes paste here), then shortens any
// bare URL the paste dropped in — the campaign-document links get a compact
// display while their full address is kept in Project.BodyLinks.
func (m *Model) pasteIntoCampaignNotes(msg tea.PasteMsg) tea.Cmd {
	p := m.findProject(m.bodyOwnerID)
	if p == nil {
		return nil
	}
	if len(p.Body) == 0 {
		p.Body = []model.BodyLine{{ID: store.NewID(), Text: ""}}
		m.bodyCursor = 0
		m.bodyEditor = m.newBodyEditor("")
	}
	start, end := m.pasteBodyLines(msg.Content)
	for i := start; i <= end && i < len(p.Body); i++ {
		if i < 0 {
			continue
		}
		if stripped, changed := shortenLinksInto(&p.BodyLinks, p.Body[i].Text); changed {
			p.Body[i].Text = stripped
		}
	}
	// Reseed the editor from the (possibly shortened) current line, caret at end.
	if m.bodyCursor >= 0 && m.bodyCursor < len(p.Body) {
		ed := m.newBodyEditor(p.Body[m.bodyCursor].Text)
		ed.CursorEnd()
		m.bodyEditor = ed
	}
	m.save()
	m.syncNoteCursor()
	return nil
}

// pruneTrailingEmptyNotes drops blank note lines off the end of a campaign's
// body when the caret leaves the notes — so a note session that ends on an
// empty line (or a placeholder that was never typed into) doesn't persist a
// dangling blank row. Interior blank lines (paragraph breaks) are kept.
func (m *Model) pruneTrailingEmptyNotes(projectID string) {
	p := m.findProject(projectID)
	if p == nil {
		return
	}
	n := len(p.Body)
	for n > 0 && strings.TrimSpace(p.Body[n-1].Text) == "" {
		n--
	}
	if n != len(p.Body) {
		p.Body = p.Body[:n]
		m.save()
	}
}
