package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

type ModalKind int

const (
	ModalQuestDetail ModalKind = iota
	ModalCampaignDetail
	ModalSectionDetail
	ModalProjectPicker
	ModalAgentPicker
	ModalHelp
	ModalDetailHelp
)

type pickerItem struct {
	ID    string
	Label string
}

// isFocusModal reports whether kind is one of the full-screen focused views
// (quest/campaign/section detail) rather than a small centered dialog — see
// renderFocusView vs renderModal.
func isFocusModal(k ModalKind) bool {
	return k == ModalQuestDetail || k == ModalCampaignDetail || k == ModalSectionDetail
}

type Modal struct {
	Kind ModalKind

	// ModalQuestDetail / ModalCampaignDetail share the body-outline editor —
	// QuestID or CampaignID is set depending on Kind (see currentBody).
	QuestID    string
	CampaignID string
	BodyCursor int
	BodyEditor textinput.Model

	// ModalCampaignDetail only: browsing the quest list below the
	// description. While true, m.cursor/m.editor target whatever quest (or
	// the "+ New Quest" row) is highlighted there — reusing the exact same
	// mechanism, and the exact same actions (handleRowKey), as the main
	// outline uses for quest rows.
	InQuestList bool

	// ModalProjectPicker
	TargetQuestID string
	PickerItems   []pickerItem
	PickerIndex   int
	PickerFilter  string // fuzzy-search query typed into the picker
	SourceRowIdx  int    // the moved quest's row index in the source list, to relocate the cursor after the move

	// ModalSectionDetail: which section ("inbox" | "someday") this page shows.
	Section string
}

// campaignQuestRows is the row list scoped to one campaign's quest section —
// its quests, in outline order, plus a trailing "+ New Quest" — used to
// navigate and delete within a campaign's focused quest list without
// spilling into the rest of the outline.
func campaignQuestRows(s *store.Store, campaignID string) []ui.Row {
	quests := ui.QuestsForCampaign(s, campaignID)
	rows := make([]ui.Row, 0, len(quests)+1)
	for _, q := range quests {
		rows = append(rows, ui.Row{Kind: ui.RowQuest, ProjectID: campaignID, QuestID: q.ID})
	}
	rows = append(rows, ui.Row{Kind: ui.RowNewQuest, ProjectID: campaignID})
	return rows
}

// sectionRows is the navigable row list for a section's focused page: the
// Questboard's quests plus a "+ New Quest" affordance, or the Vault's parked
// quests followed by its archived campaigns.
func sectionRows(s *store.Store, section string) []ui.Row {
	var rows []ui.Row
	switch section {
	case "inbox":
		for _, q := range ui.QuestsForInbox(s) {
			rows = append(rows, ui.Row{Kind: ui.RowQuest, QuestID: q.ID})
		}
		rows = append(rows, ui.Row{Kind: ui.RowNewQuest})
	case "someday":
		for _, q := range ui.QuestsForSomeday(s) {
			rows = append(rows, ui.Row{Kind: ui.RowQuest, ProjectID: q.ProjectID, QuestID: q.ID, ShowProjectTag: q.ProjectID != ""})
		}
		for _, p := range s.Projects {
			if p.Archived {
				rows = append(rows, ui.Row{Kind: ui.RowProject, ProjectID: p.ID})
			}
		}
	}
	return rows
}

// filteredPickerItems is the project-picker list narrowed to the fuzzy filter
// typed so far (case-insensitive subsequence match); the full list when empty.
func (mod *Modal) filteredPickerItems() []pickerItem {
	if mod.PickerFilter == "" {
		return mod.PickerItems
	}
	var out []pickerItem
	for _, it := range mod.PickerItems {
		if fuzzySubsequence(mod.PickerFilter, it.Label) {
			out = append(out, it)
		}
	}
	return out
}

// fuzzySubsequence reports whether every rune of query appears in target in
// order (case-insensitive) — the classic fuzzy-finder match.
func fuzzySubsequence(query, target string) bool {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return true
	}
	qi := 0
	for _, tc := range strings.ToLower(target) {
		if tc == q[qi] {
			if qi++; qi == len(q) {
				return true
			}
		}
	}
	return false
}

func sectionDetailModal(section string) *Modal {
	return &Modal{Kind: ModalSectionDetail, Section: section}
}

// sectionTitle is the heading + count shown atop a section's focused page.
func (m *Model) sectionTitle(section string) string {
	switch section {
	case "inbox":
		return fmt.Sprintf("Questboard (%d)", ui.CountInbox(m.store))
	case "someday":
		return fmt.Sprintf("Vault (%d)", ui.CountSomeday(m.store)+ui.CountArchived(m.store))
	}
	return section
}

func bodyLineEditor(text string) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(text)
	ti.CursorEnd()
	_ = ti.Focus()
	return ti
}

// newBodyEditor is bodyLineEditor plus clearing any active selection —
// used wherever mod.BodyEditor is replaced mid-session (moving between
// body lines, inserting/removing one), so a selection from the line just
// left behind can't appear to apply to the new one.
func (m *Model) newBodyEditor(text string) textinput.Model {
	m.clearSelection()
	return bodyLineEditor(text)
}

func questDetailModal(q *model.Quest) *Modal {
	if len(q.Body) == 0 {
		q.Body = []model.BodyLine{{ID: store.NewID(), Text: ""}}
	}
	return &Modal{
		Kind:       ModalQuestDetail,
		QuestID:    q.ID,
		BodyCursor: 0,
		BodyEditor: bodyLineEditor(q.Body[0].Text),
	}
}

func campaignDetailModal(p *model.Project) *Modal {
	if len(p.Body) == 0 {
		p.Body = []model.BodyLine{{ID: store.NewID(), Text: ""}}
	}
	return &Modal{
		Kind:       ModalCampaignDetail,
		CampaignID: p.ID,
		BodyCursor: 0,
		BodyEditor: bodyLineEditor(p.Body[0].Text),
	}
}

func projectPickerModal(s *store.Store, questID, currentProjectID string) *Modal {
	items := []pickerItem{{ID: "", Label: "Questboard (no campaign)"}}
	idx := 0
	for _, p := range s.Projects {
		items = append(items, pickerItem{ID: p.ID, Label: p.Name})
		if p.ID == currentProjectID {
			idx = len(items) - 1
		}
	}
	return &Modal{Kind: ModalProjectPicker, TargetQuestID: questID, PickerItems: items, PickerIndex: idx}
}

func helpModal() *Modal {
	return &Modal{Kind: ModalHelp}
}

func detailHelpModal() *Modal {
	return &Modal{Kind: ModalDetailHelp}
}

// currentBody returns a pointer into the store's own slice for whichever
// entity (quest or campaign) owns the open modal's body outline, so edits
// through it always persist. Shared by ModalQuestDetail and
// ModalCampaignDetail so the outline-editing logic below only exists once.
func (m *Model) currentBody() *[]model.BodyLine {
	mod := m.modal
	if mod == nil {
		return nil
	}
	switch mod.Kind {
	case ModalQuestDetail:
		if q := m.findQuest(mod.QuestID); q != nil {
			return &q.Body
		}
	case ModalCampaignDetail:
		if p := m.findProject(mod.CampaignID); p != nil {
			return &p.Body
		}
	}
	return nil
}

func (m *Model) touchBodyOwner() {
	mod := m.modal
	if mod == nil {
		return
	}
	if mod.Kind == ModalQuestDetail {
		if q := m.findQuest(mod.QuestID); q != nil {
			q.UpdatedAt = time.Now()
		}
	}
	m.save()
}

func (m *Model) commitBodyLine() {
	mod := m.modal
	body := m.currentBody()
	if body == nil || mod.BodyCursor < 0 || mod.BodyCursor >= len(*body) {
		return
	}
	(*body)[mod.BodyCursor].Text = mod.BodyEditor.Value()
	m.touchBodyOwner()
}

// bodyVisualRow is one on-screen row of the wrapped body: which body line
// it belongs to and the [start,end) raw-rune range it covers.
type bodyVisualRow struct {
	line       int
	start, end int
}

func (m *Model) focusWrapWidth() int {
	w := m.focusTextWidth
	if w < 8 {
		w = 8 // matches renderBodyLineWrapped's floor
	}
	return w
}

// bodyVisualRows wraps every body line at the focus width into the flat
// list of on-screen rows the focus view renders — the basis for vertical
// (Up/Down) movement, which must step one visual row at a time or wrapped
// lines get skipped over.
func (m *Model) bodyVisualRows() []bodyVisualRow {
	body := m.currentBody()
	if body == nil {
		return nil
	}
	width := m.focusWrapWidth()
	var out []bodyVisualRow
	for li := range *body {
		for _, seg := range wrapSegments([]rune((*body)[li].Text), width) {
			out = append(out, bodyVisualRow{line: li, start: seg[0], end: seg[1]})
		}
	}
	return out
}

// currentVisualRow finds the cursor's row in rows; a position sitting on a
// wrap boundary resolves to the later row (where typing would continue).
func (m *Model) currentVisualRow(rows []bodyVisualRow) int {
	mod := m.modal
	cur := mod.BodyEditor.Position()
	found := -1
	for k, vr := range rows {
		if vr.line == mod.BodyCursor && cur >= vr.start && cur <= vr.end {
			found = k
		}
	}
	return found
}

// moveBodyCursor moves the cursor one visual row up (delta<0) or down
// (delta>0), preserving the visual column and committing the current line
// first. Returns false when already at the top/bottom visual row of the
// body, so a caller can hand off (campaign detail drops into its quest
// list off the bottom).
func (m *Model) moveBodyCursor(delta int) bool {
	body := m.currentBody()
	if body == nil {
		return false
	}
	m.commitBodyLine()
	m.clearSelection() // a plain vertical move drops any selection
	mod := m.modal

	rows := m.bodyVisualRows()
	cur := m.currentVisualRow(rows)
	if cur < 0 {
		return false
	}
	target := cur + delta
	if target < 0 || target >= len(rows) {
		return false
	}

	vcol := mod.BodyEditor.Position() - rows[cur].start
	tr := rows[target]
	maxPos := tr.end
	if target+1 < len(rows) && rows[target+1].line == tr.line {
		maxPos = tr.end - 1 // not the last row of its line: end is the next row's start, stay on this one
	}
	pos := clampInt(tr.start+vcol, tr.start, maxPos)

	if tr.line == mod.BodyCursor {
		mod.BodyEditor.SetCursor(pos)
	} else {
		m.seedBodyEditor(tr.line, pos)
	}
	return true
}

// seedBodyEditor points the editor at body line idx with the cursor at col,
// clearing any selection.
func (m *Model) seedBodyEditor(idx, col int) {
	mod := m.modal
	body := m.currentBody()
	mod.BodyCursor = idx
	ed := m.newBodyEditor((*body)[idx].Text)
	ed.SetCursor(col)
	mod.BodyEditor = ed
}

// handleBodyOutlineKey handles the body-outline editing keys shared by
// ModalQuestDetail and ModalCampaignDetail — the line split/merge/exit
// behaviors of a normal multiline editor (modeled on Obsidian/Notion list
// editing), plus Ctrl+D objective toggling and multiline paste.
// handled=false means the caller should forward msg to the line editor as
// ordinary text input.
func (m *Model) handleBodyOutlineKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	mod := m.modal
	body := m.currentBody()
	if body == nil {
		return nil, false
	}

	switch {
	case msg.Type == tea.KeyEnter:
		raw := []rune(mod.BodyEditor.Value())
		pos := mod.BodyEditor.Position()
		kind, display := model.ClassifyBodyLine(string(raw))

		// Enter on an empty "- "/"# " line exits the list/heading — the
		// marker clears instead of yet another marked line appearing.
		if kind != model.BodyText && strings.TrimSpace(display) == "" {
			(*body)[mod.BodyCursor].Text = ""
			(*body)[mod.BodyCursor].Done = false
			m.touchBodyOwner()
			m.seedBodyEditor(mod.BodyCursor, 0)
			return nil, true
		}

		// Split the line at the cursor: everything after it moves to a new
		// line below. Splitting inside an objective's content continues the
		// list ("- " carries onto the new line); headings and plain text
		// split plainly.
		left, right := string(raw[:pos]), string(raw[pos:])
		newCol := 0
		if kind == model.BodyObjective && pos >= 2 {
			right = "- " + strings.TrimLeft(right, " ")
			newCol = 2
		}
		indent := (*body)[mod.BodyCursor].Indent // the new line keeps the same nesting
		(*body)[mod.BodyCursor].Text = left
		insertAt := mod.BodyCursor + 1
		*body = append(*body, model.BodyLine{})
		copy((*body)[insertAt+1:], (*body)[insertAt:])
		(*body)[insertAt] = model.BodyLine{ID: store.NewID(), Text: right, Indent: indent}
		m.touchBodyOwner()
		m.seedBodyEditor(insertAt, newCol)
		return nil, true

	case msg.Type == tea.KeyBackspace:
		if mod.BodyEditor.Position() != 0 {
			return nil, false // normal in-line character delete
		}
		raw := mod.BodyEditor.Value()
		kind, display := model.ClassifyBodyLine(raw)
		if kind != model.BodyText {
			// First Backspace at the start of a marked line just strips the
			// marker (the line becomes plain text); the next one merges.
			(*body)[mod.BodyCursor].Text = display
			(*body)[mod.BodyCursor].Done = false
			m.touchBodyOwner()
			m.seedBodyEditor(mod.BodyCursor, 0)
			return nil, true
		}
		if mod.BodyCursor == 0 {
			return nil, true // nothing above to merge into
		}
		prevIdx := mod.BodyCursor - 1
		junction := len([]rune((*body)[prevIdx].Text))
		(*body)[prevIdx].Text += raw
		*body = append((*body)[:mod.BodyCursor], (*body)[mod.BodyCursor+1:]...)
		m.touchBodyOwner()
		m.seedBodyEditor(prevIdx, junction)
		return nil, true

	case msg.Type == tea.KeyDelete:
		raw := []rune(mod.BodyEditor.Value())
		if mod.BodyEditor.Position() < len(raw) || mod.BodyCursor >= len(*body)-1 {
			return nil, false // normal forward delete / nothing below
		}
		// Forward-merge: pull the next line up, dropping its marker (its
		// bullet/heading prefix would otherwise land mid-line as literal
		// "- " text).
		_, nextDisplay := model.ClassifyBodyLine((*body)[mod.BodyCursor+1].Text)
		(*body)[mod.BodyCursor].Text = string(raw) + nextDisplay
		*body = append((*body)[:mod.BodyCursor+1], (*body)[mod.BodyCursor+2:]...)
		m.touchBodyOwner()
		m.seedBodyEditor(mod.BodyCursor, len(raw))
		return nil, true

	case msg.Type == tea.KeyRunes && strings.ContainsAny(string(msg.Runes), "\n\r"):
		// A multi-line paste — split it into real body lines instead of
		// letting textinput collapse the newlines into spaces. Any links in the
		// pasted lines (and ONLY those lines) are captured immediately.
		start, end := m.pasteBodyLines(string(msg.Runes))
		if mod.Kind == ModalQuestDetail {
			if q := m.findQuest(mod.QuestID); q != nil {
				return m.captureBodyLinesRange(q, start, end), true
			}
		}
		return nil, true

	case msg.Type == tea.KeyTab:
		m.indentBodyLine(1)
		return nil, true

	case msg.Type == tea.KeyShiftTab:
		m.indentBodyLine(-1)
		return nil, true

	case msg.String() == "ctrl+d":
		m.commitBodyLine()
		body = m.currentBody()
		kind, _ := model.ClassifyBodyLine((*body)[mod.BodyCursor].Text)
		if kind == model.BodyObjective {
			(*body)[mod.BodyCursor].Done = !(*body)[mod.BodyCursor].Done
			m.touchBodyOwner()
		}
		mod.BodyEditor = m.newBodyEditor((*body)[mod.BodyCursor].Text)
		return nil, true
	}

	return nil, false
}

// indentBodyLine nudges the current line's nesting in or out by one level.
// Indenting is capped at one level deeper than the line above (so you can't
// create an orphan gap); outdenting floors at 0. The line's text and the
// caret column are untouched.
func (m *Model) indentBodyLine(delta int) {
	mod := m.modal
	m.commitBodyLine()
	body := m.currentBody()
	if body == nil {
		return
	}
	i := mod.BodyCursor
	next := (*body)[i].Indent + delta
	if next < 0 {
		next = 0
	}
	if delta > 0 {
		max := 0
		if i > 0 {
			max = (*body)[i-1].Indent + 1
		}
		if next > max {
			next = max
		}
	}
	if next == (*body)[i].Indent {
		return
	}
	(*body)[i].Indent = next
	m.touchBodyOwner()
}

// pasteBodyLines inserts pasted multi-line text at the cursor: the first
// pasted line joins the text before the cursor, the rest become their own
// body lines, and whatever followed the cursor ends up after the final
// pasted line — standard editor paste semantics. Returns the [start, end]
// range of body-line indices the paste touched, so link capture can scan only
// those lines (never pre-existing inline references elsewhere in the body).
func (m *Model) pasteBodyLines(text string) (start, end int) {
	mod := m.modal
	body := m.currentBody()

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	chunks := strings.Split(text, "\n")

	raw := []rune(mod.BodyEditor.Value())
	pos := mod.BodyEditor.Position()
	left, right := string(raw[:pos]), string(raw[pos:])

	start = mod.BodyCursor
	indent := (*body)[mod.BodyCursor].Indent // pasted lines keep the current nesting
	(*body)[mod.BodyCursor].Text = left + chunks[0]
	insertAt := mod.BodyCursor + 1
	for i := 1; i < len(chunks); i++ {
		line := model.BodyLine{ID: store.NewID(), Text: chunks[i], Indent: indent}
		*body = append(*body, model.BodyLine{})
		copy((*body)[insertAt+1:], (*body)[insertAt:])
		(*body)[insertAt] = line
		insertAt++
	}
	lastIdx := insertAt - 1
	endCol := len([]rune((*body)[lastIdx].Text))
	(*body)[lastIdx].Text += right
	m.touchBodyOwner()
	m.seedBodyEditor(lastIdx, endCol)
	return start, lastIdx
}

// focusScrollBy moves the focus-view caret n rows up/down by replaying that
// many Up/Down key presses through updateModal — so wheel and Page keys
// reuse the exact navigation logic (body rows, quest-list transitions), and
// the view's caret-driven scroll follows along.
func (m *Model) focusScrollBy(down bool, n int) tea.Cmd {
	k := tea.KeyMsg{Type: tea.KeyUp}
	if down {
		k.Type = tea.KeyDown
	}
	var cmd tea.Cmd
	for i := 0; i < n; i++ {
		cmd = m.updateModal(k)
	}
	return cmd
}

func (m *Model) updateModal(msg tea.KeyMsg) tea.Cmd {
	mod := m.modal

	// PageUp/PageDown scroll a focused quest/campaign by half a screen.
	if isFocusModal(mod.Kind) && (msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown) {
		half := m.height / 2
		if half < 1 {
			half = 1
		}
		return m.focusScrollBy(msg.Type == tea.KeyPgDown, half)
	}

	switch mod.Kind {
	case ModalHelp, ModalDetailHelp:
		m.closeModal()
		return nil

	case ModalProjectPicker:
		items := mod.filteredPickerItems()
		switch msg.String() {
		case "up":
			if mod.PickerIndex > 0 {
				mod.PickerIndex--
			}
		case "down":
			if mod.PickerIndex < len(items)-1 {
				mod.PickerIndex++
			}
		case "enter":
			if len(items) > 0 {
				if target := m.findQuest(mod.TargetQuestID); target != nil {
					target.ProjectID = items[mod.PickerIndex].ID
					target.UpdatedAt = time.Now()
					m.save()
				}
			}
			// Relocate the cursor to the source list's next item (or previous
			// if it was last) rather than following the quest into its new
			// home — see SourceRowIdx.
			srcIdx := mod.SourceRowIdx
			m.closeModal()
			if row, ok := nearestSelectableRow(m.currentRowScope(), srcIdx); ok {
				m.setCursor(row)
			}
		case "esc":
			m.closeModal()
		case "backspace":
			if r := []rune(mod.PickerFilter); len(r) > 0 {
				mod.PickerFilter = string(r[:len(r)-1])
				mod.PickerIndex = 0
			}
		default:
			if msg.Type == tea.KeyRunes {
				mod.PickerFilter += string(msg.Runes)
				mod.PickerIndex = 0
			}
		}
		return nil

	case ModalAgentPicker:
		items := mod.filteredPickerItems()
		switch msg.String() {
		case "up":
			if mod.PickerIndex > 0 {
				mod.PickerIndex--
			}
		case "down":
			if mod.PickerIndex < len(items)-1 {
				mod.PickerIndex++
			}
		case "enter":
			var cmd tea.Cmd
			if len(items) > 0 {
				if target := m.findQuest(mod.TargetQuestID); target != nil {
					wt := items[mod.PickerIndex].ID
					if indexOfStr(target.AgentWorktrees, wt) < 0 {
						target.AgentWorktrees = append(target.AgentWorktrees, wt)
					}
					target.UpdatedAt = time.Now()
					m.save()
					// Reflect the pinned agent immediately, and make sure the
					// poll floor is running now that a worktree is pinned.
					cmd = tea.Batch(refreshAgentsCmd(), m.maybeStartAgentPoll())
				}
			}
			m.closeModal()
			return cmd
		case "esc":
			m.closeModal()
		case "backspace":
			if r := []rune(mod.PickerFilter); len(r) > 0 {
				mod.PickerFilter = string(r[:len(r)-1])
				mod.PickerIndex = 0
			}
		default:
			if msg.Type == tea.KeyRunes {
				mod.PickerFilter += string(msg.Runes)
				mod.PickerIndex = 0
			}
		}
		return nil

	case ModalQuestDetail:
		q := m.findQuest(mod.QuestID)
		if q == nil {
			m.closeModal()
			return nil
		}
		// The link cursor (Jira/PR lines above the body) owns navigation when
		// it's active — Enter opens, Ctrl+X arms removal, up/down step through
		// the links and hand back to the body off the bottom.
		if m.onFocusLink() {
			if cmd, handled := m.handleFocusLinkKey(msg, q); handled {
				return cmd
			}
		}
		if msg.Type == tea.KeyEsc {
			m.commitBodyLine()
			m.closeModal()
			return nil
		}
		if handled, cmd := m.applyBodySelectionKey(msg); handled {
			return cmd
		}
		if msg.String() == "up" {
			// Off the top of the body, step onto the bottom-most link line.
			if !m.moveBodyCursor(-1) && m.integrationsEnabled && m.focusLinkCount(q) > 0 {
				m.focusLinkIdx = m.focusLinkCount(q) - 1
			}
			return nil
		}
		if msg.String() == "down" {
			m.moveBodyCursor(1)
			return nil
		}
		if cmd, handled := m.handleBodyOutlineKey(msg); handled {
			return cmd
		}
		var cmd tea.Cmd
		mod.BodyEditor, cmd = mod.BodyEditor.Update(msg)
		// After an ordinary edit, if the current line now holds a complete
		// Jira/PR URL, capture it, strip it from the line, and fire an
		// immediate sync for just the new code(s).
		if syncCmd := m.captureCurrentBodyLink(q); syncCmd != nil {
			return tea.Batch(cmd, syncCmd)
		}
		return cmd

	case ModalCampaignDetail:
		p := m.findProject(mod.CampaignID)
		if p == nil {
			m.closeModal()
			return nil
		}
		if msg.Type == tea.KeyEsc {
			m.commitBodyLine()
			m.commitEdit()
			// Always return to the campaign itself in the outline below,
			// regardless of whether the description or the quest list was
			// focused when closing.
			m.setCursor(ui.Row{Kind: ui.RowProject, ProjectID: p.ID})
			m.closeModal()
			return nil
		}

		if mod.InQuestList {
			rows := campaignQuestRows(m.store, p.ID)
			switch msg.String() {
			case "up":
				idx := findRowIndex(rows, m.cursor)
				if idx <= 0 {
					mod.InQuestList = false
					m.editor = nil
					body := m.currentBody()
					mod.BodyCursor = len(*body) - 1
					mod.BodyEditor = m.newBodyEditor((*body)[mod.BodyCursor].Text)
					return nil
				}
				m.commitEdit()
				m.setCursor(rows[idx-1])
				return nil
			case "down":
				idx := findRowIndex(rows, m.cursor)
				if idx >= 0 && idx < len(rows)-1 {
					m.commitEdit()
					m.setCursor(rows[idx+1])
				}
				return nil
			}
			return m.handleRowKey(msg)
		}

		if handled, cmd := m.applyBodySelectionKey(msg); handled {
			return cmd
		}

		if msg.String() == "down" {
			// Only drop into the quest list off the last VISUAL row of the
			// body — a wrapped last line steps through its rows first.
			if !m.moveBodyCursor(1) {
				mod.InQuestList = true
				rows := campaignQuestRows(m.store, p.ID)
				m.setCursor(rows[0])
			}
			return nil
		}
		if msg.String() == "up" {
			m.moveBodyCursor(-1)
			return nil
		}
		if cmd, handled := m.handleBodyOutlineKey(msg); handled {
			return cmd
		}
		var cmd tea.Cmd
		mod.BodyEditor, cmd = mod.BodyEditor.Update(msg)
		return cmd

	case ModalSectionDetail:
		if msg.Type == tea.KeyEsc {
			m.commitEdit()
			m.setCursor(ui.Row{Kind: ui.RowSection, Section: mod.Section})
			m.closeModal()
			return nil
		}
		rows := sectionRows(m.store, mod.Section)
		switch msg.String() {
		case "up":
			if idx := findRowIndex(rows, m.cursor); idx > 0 {
				m.commitEdit()
				m.setCursor(rows[idx-1])
			}
			return nil
		case "down":
			if idx := findRowIndex(rows, m.cursor); idx >= 0 && idx < len(rows)-1 {
				m.commitEdit()
				m.setCursor(rows[idx+1])
			}
			return nil
		}
		return m.handleRowKey(msg)
	}

	return nil
}

func (m *Model) renderModal() string {
	mod := m.modal
	var content string

	switch mod.Kind {
	case ModalHelp:
		var b strings.Builder
		b.WriteString(ui.StyleTitle.Render("Quests"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("A single fluid outline for tracking quests inside campaigns."))
		b.WriteString("\n\n")

		b.WriteString(ui.StyleSectionHeader.Render("Terms"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-11s%s\n", "Tavern", ui.StyleMuted.Render("home base — the full outline: Questboard, campaigns, Vault"))
		fmt.Fprintf(&b, "%-11s%s\n", "Wilds", ui.StyleMuted.Render("focused view (Ctrl+G) — just your taken-up quests to work through"))
		fmt.Fprintf(&b, "%-11s%s\n", "Questboard", ui.StyleMuted.Render("your inbox — new quests with no campaign yet"))
		fmt.Fprintf(&b, "%-11s%s\n", "Campaigns", ui.StyleMuted.Render("your projects — each lists its own quests"))
		fmt.Fprintf(&b, "%-11s%s\n", "Vault", ui.StyleMuted.Render("your archive — parked quests and retired campaigns"))
		fmt.Fprintf(&b, "%-11s%s\n", "Take up", ui.StyleMuted.Render("mark a quest active (Ctrl+A) to bring it Wilds"))
		b.WriteString("\n")

		b.WriteString(ui.StyleSectionHeader.Render("Data"))
		b.WriteString("\n")
		dataPath, err := store.DefaultPath()
		if err != nil {
			dataPath = "~/.config/quests/data.json"
		}
		b.WriteString(ui.StyleMuted.Render("Saved locally, no account or sync: " + dataPath))
		b.WriteString("\n")
		if bdir, err := store.BackupsDir(); err == nil {
			line := "Daily backups in " + bdir
			if date, ok := store.LatestBackup(bdir); ok {
				line += " (last: " + date + ")"
			} else {
				line += " (none yet)"
			}
			b.WriteString(ui.StyleMuted.Render(line))
			b.WriteString("\n")
		}
		b.WriteString("\n")

		b.WriteString(ui.StyleSectionHeader.Render("Integrations"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("Paste a Jira or GitHub PR URL into a quest's body to link it (multiple of"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("each allowed). On paste it's tracked and the URL is shortened inline to"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("its code, still clickable. Click a code to open it; arrow onto a link in"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("the expanded view to open (↵) or remove (" + Keys.Delete.Help().Key + ") it. PR shows " + ui.GlyphPRSuccess + "/" + ui.GlyphPRError + "/" + ui.GlyphPRRunning + "/" + ui.GlyphPRMerged + ","))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("+ resolved/total comments; Jira shows " + ui.GlyphJiraTodo + "/" + ui.GlyphJiraInProgress + "/" + ui.GlyphJiraDone + " (todo / wip / done), ~60s."))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("Requires gh and acli authenticated locally (gh auth login; acli jira"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("auth login)."))
		b.WriteString("\n\n")

		b.WriteString(ui.StyleSectionHeader.Render("Keys"))
		b.WriteString("\n")
		for _, group := range Keys.FullHelp() {
			for _, kb := range group {
				h := kb.Help()
				if h.Key == "" {
					continue
				}
				fmt.Fprintf(&b, "%-11s%s\n", h.Key, h.Desc)
			}
			b.WriteString("\n")
		}
		b.WriteString(ui.StyleMuted.Render("press any key to close"))
		content = strings.TrimRight(b.String(), "\n")

	case ModalProjectPicker:
		var b strings.Builder
		b.WriteString(ui.StyleTitle.Render("Move to campaign"))
		b.WriteString("\n")
		query := mod.PickerFilter
		if query == "" {
			query = ui.StyleMuted.Render("type to filter…")
		}
		b.WriteString(ui.StyleMuted.Render("› ") + query + "\n\n")
		items := mod.filteredPickerItems()
		if len(items) == 0 {
			b.WriteString(ui.StyleMuted.Render("  (no matching campaigns)") + "\n")
		}
		for i, item := range items {
			line := "  " + item.Label
			if i == mod.PickerIndex {
				line = ui.StyleSelectedRow.Render("> " + item.Label)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + ui.StyleMuted.Render("type to filter · ↑↓ choose · enter confirm · esc cancel"))
		content = b.String()

	case ModalAgentPicker:
		var b strings.Builder
		b.WriteString(ui.StyleTitle.Render("Pin a Claude agent"))
		b.WriteString("\n")
		query := mod.PickerFilter
		if query == "" {
			query = ui.StyleMuted.Render("type to filter…")
		}
		b.WriteString(ui.StyleMuted.Render("› ") + query + "\n\n")
		items := mod.filteredPickerItems()
		if len(items) == 0 {
			b.WriteString(ui.StyleMuted.Render("  (no Claude agents running)") + "\n")
		}
		for i, item := range items {
			line := "  " + item.Label
			if i == mod.PickerIndex {
				line = ui.StyleSelectedRow.Render("> " + item.Label)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + ui.StyleMuted.Render("type to filter · ↑↓ choose · enter pin · esc cancel"))
		content = b.String()

	case ModalDetailHelp:
		var b strings.Builder
		b.WriteString(ui.StyleTitle.Render("Quest & campaign details"))
		b.WriteString("\n\n")

		b.WriteString(ui.StyleSectionHeader.Render("Formatting"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-11s%s\n", `"# "`, ui.StyleMuted.Render("start a heading"))
		fmt.Fprintf(&b, "%-11s%s\n", `"- "`, ui.StyleMuted.Render("start an objective (checkbox)"))
		fmt.Fprintf(&b, "%-11s%s\n", "Ctrl+D", ui.StyleMuted.Render("toggle the objective under the cursor"))
		fmt.Fprintf(&b, "%-11s%s\n", "Tab / ⇧Tab", ui.StyleMuted.Render("indent / outdent the line (nest objectives)"))
		fmt.Fprintf(&b, "%-11s%s\n", "Enter", ui.StyleMuted.Render("inside a list \"- \" carries onto the next line; empty, it exits the list"))
		fmt.Fprintf(&b, "%-11s%s\n", "Paste", ui.StyleMuted.Render("multi-line text splits into real lines (\"- \" / \"# \" recognized)"))
		b.WriteString("\n")

		b.WriteString(ui.StyleSectionHeader.Render("Selecting text"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-11s%s\n", "Shift+←/→", ui.StyleMuted.Render("select character by character (copies as it grows)"))
		fmt.Fprintf(&b, "%-11s%s\n", "Shift+↑/↓", ui.StyleMuted.Render("extend the selection across lines (copy-only)"))
		fmt.Fprintf(&b, "%-11s%s\n", "Mouse", ui.StyleMuted.Render("click a line and drag, across lines too"))
		b.WriteString("\n")

		b.WriteString(ui.StyleSectionHeader.Render("Integration links (quest detail)"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-11s%s\n", "Paste URL", ui.StyleMuted.Render("a Jira/PR URL is captured instantly and pulled out of the line"))
		fmt.Fprintf(&b, "%-11s%s\n", "↑ from body", ui.StyleMuted.Render("steps onto the link lines above; ↓ returns to the body"))
		fmt.Fprintf(&b, "%-11s%s\n", "Enter", ui.StyleMuted.Render("open the focused link in the browser (agents show status only)"))
		fmt.Fprintf(&b, "%-11s%s\n", "Ctrl+X", ui.StyleMuted.Render("remove the focused link / unpin the agent (inline y/n)"))
		fmt.Fprintf(&b, "%-11s%s\n", "+ add agent", ui.StyleMuted.Render("the muted line below the links pins a Claude agent (↵ / click)"))
		b.WriteString("\n")

		b.WriteString(ui.StyleSectionHeader.Render("Campaign quest list"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-11s%s\n", "Tab", ui.StyleMuted.Render("open a listed quest's own detail view"))
		fmt.Fprintf(&b, "%-11s%s\n", "Enter", ui.StyleMuted.Render("add a quest via \"+ New Quest\", or a sibling below one"))
		fmt.Fprintf(&b, "%-11s%s\n", "Ctrl+D/G/T", ui.StyleMuted.Render("toggle done / active-taken / type on the highlighted quest"))
		fmt.Fprintf(&b, "%-11s%s\n", "Ctrl+P", ui.StyleMuted.Render("move the highlighted quest to a different campaign"))
		fmt.Fprintf(&b, "%-11s%s\n", "Ctrl+X", ui.StyleMuted.Render("delete the highlighted quest (inline y/n)"))
		fmt.Fprintf(&b, "%-11s%s\n", "Shift+↑/↓", ui.StyleMuted.Render("reorder the highlighted quest"))
		b.WriteString("\n")

		b.WriteString(ui.StyleMuted.Render("press any key to close"))
		content = strings.TrimRight(b.String(), "\n")
	}

	boxWidth := 64
	if mod.Kind == ModalHelp || mod.Kind == ModalDetailHelp {
		boxWidth = 86
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorAccent).
		Padding(1, 2).
		Width(minInt(boxWidth, m.width-4)).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// renderFocusView takes over the whole screen with just the quest or
// campaign under Tab — not a boxed dialog, a focused view of that one
// thing. "← back (esc)" is the only way out; every other interaction
// (editing, adding/removing quests, toggling done, etc.) works exactly as
// it does in the main outline.
func (m *Model) renderFocusView() string {
	// Leave comfortable breathing room: at least ~4 columns each side, and
	// a blank row top and bottom (see vpad below).
	contentWidth := m.width - 8
	if contentWidth > 80 {
		contentWidth = 80
	}
	if contentWidth < 20 {
		contentWidth = 20
	}
	leftMargin := (m.width - contentWidth) / 2
	if leftMargin < 0 {
		leftMargin = 0
	}
	margin := strings.Repeat(" ", leftMargin)

	back := ui.StyleMuted.Render("← back (esc)")
	right := ui.StyleMuted.Render("F1 help")
	if m.clipboardToastActive {
		right = renderClipboardToast()
	}
	pad := contentWidth - lipgloss.Width(back) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	header := back + strings.Repeat(" ", pad) + right

	// The wrap width and margin must be set BEFORE renderFocusContent runs —
	// it soft-wraps body lines against focusTextWidth.
	m.focusLeftMargin = leftMargin
	m.focusTextWidth = contentWidth - 4 // body text starts 4 cols in (cursor mark + glyph slot)

	content := header + "\n\n" + m.renderFocusContent()
	lines := strings.Split(content, "\n")

	// When it all fits, center it vertically (as before). When it doesn't,
	// scroll: keep the caret's line on screen, letting the header scroll off
	// the top rather than shearing the layout. allLines[0]=header,
	// [1]=blank, then renderFocusContent — so the caret's line is at
	// 2 + focusCaretLine, and body row 0 is at index 4.
	// Keep generous breathing room top and bottom, but never more than half
	// the screen so short terminals stay usable.
	vpad := viewVPad
	if maxPad := m.height / 4; vpad > maxPad {
		vpad = maxPad
	}
	if vpad < 0 {
		vpad = 0
	}
	avail := m.height - 2*vpad
	if avail < 1 {
		avail = 1
	}
	topPad, scroll := vpad, 0
	if len(lines) <= avail {
		topPad = vpad + (avail-len(lines))/2
		m.focusScroll = 0
	} else {
		caretAbs := 2 + m.focusCaretLine
		switch {
		case caretAbs < avail:
			m.focusScroll = 0 // caret in the first screenful: show from the top (header visible)
		case caretAbs < m.focusScroll:
			m.focusScroll = caretAbs // scrolled above the window: bring it to the top edge
		case caretAbs >= m.focusScroll+avail:
			m.focusScroll = caretAbs - avail + 1 // below the window: bring it to the bottom edge
		}
		maxScroll := len(lines) - avail
		m.focusScroll = clampInt(m.focusScroll, 0, maxScroll)
		scroll = m.focusScroll
	}

	// Screen positions used by handleFocusMouse (shifted up by scroll). The
	// header's extents make "← back (esc)" and "F1 help" clickable (the
	// latter only while it's not swapped out for the clipboard toast).
	// Content line 0 sits two rows below topPad (the header + its blank);
	// body row 0 is focusBodyLineStart content lines further down (title,
	// integration codes, blanks push it down).
	m.focusContentTop = topPad + 2 - scroll
	m.focusBodyBaseRow = m.focusContentTop + m.focusBodyLineStart
	m.focusHeaderRow = topPad - scroll
	m.focusBackWidth = lipgloss.Width(back)
	m.focusHelpX = leftMargin + m.focusBackWidth + pad
	m.focusHelpWidth = lipgloss.Width(right)
	if m.clipboardToastActive {
		m.focusHelpWidth = 0 // the toast isn't a button
	}

	end := scroll + avail
	if end > len(lines) {
		end = len(lines)
	}

	// Safety net: clip every rendered row at the terminal edge so nothing
	// can hard-wrap and shear the layout.
	clip := lipgloss.NewStyle().MaxWidth(m.width)
	var b strings.Builder
	for i := 0; i < topPad; i++ {
		// Tuck a "more above" hint into the last row of the top padding.
		if i == topPad-1 && scroll > 0 {
			b.WriteString(foldHint(margin, contentWidth) + "\n")
			continue
		}
		b.WriteString("\n")
	}
	for _, line := range lines[scroll:end] {
		b.WriteString(clip.Render(margin+line) + "\n")
	}
	if end < len(lines) {
		b.WriteString(foldHint(margin, contentWidth) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// foldHint is the muted "···" shown at the top/bottom edge of a scrolled
// view to signal there's more content beyond the fold — left-aligned to the
// content's left edge.
func foldHint(margin string, width int) string {
	_ = width
	return margin + ui.StyleMuted.Render("···")
}

// handleFocusMouse handles the mouse inside a focused quest/campaign view:
// clicking the header's "← back (esc)"/"F1 help" triggers them, clicking a
// body line moves the editing cursor there, and dragging (including across
// lines) extends a text selection from where the press landed.
func (m *Model) handleFocusMouse(msg tea.MouseMsg) tea.Cmd {
	mod := m.modal
	if mod == nil {
		return nil
	}
	if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		return m.focusScrollBy(msg.Button == tea.MouseButtonWheelDown, 1)
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	press := msg.Action == tea.MouseActionPress
	if !press && msg.Action != tea.MouseActionMotion {
		return nil
	}

	if press && msg.Y == m.focusHeaderRow {
		switch {
		case msg.X >= m.focusLeftMargin && msg.X < m.focusLeftMargin+m.focusBackWidth:
			return m.updateModal(tea.KeyMsg{Type: tea.KeyEsc})
		case msg.X >= m.focusHelpX && msg.X < m.focusHelpX+m.focusHelpWidth:
			m.pushModal(detailHelpModal())
		}
		return nil
	}

	// A click on an integration code (Jira/PR) opens its URL; a click on the
	// "+ add Claude agent" affordance opens the picker.
	if press {
		for _, sp := range m.focusCodeSpans {
			if msg.Y == m.focusContentTop+sp.line && msg.X >= sp.x0 && msg.X < sp.x1 {
				if sp.url == addAgentSentinel {
					m.openAgentPicker()
					return nil
				}
				return openURL(sp.url)
			}
		}
	}

	if mod.Kind == ModalCampaignDetail && mod.InQuestList {
		return nil
	}
	body := m.currentBody()
	if body == nil {
		return nil
	}
	// Soft-wrapped body lines span several screen rows — the row map built
	// during rendering resolves a row back to its body line and the raw
	// rune offset that row starts at.
	bodyRow := msg.Y - m.focusBodyBaseRow
	if bodyRow < 0 || bodyRow >= len(m.focusRowLine) {
		return nil
	}
	bodyIdx := m.focusRowLine[bodyRow]
	if bodyIdx < 0 || bodyIdx >= len(*body) {
		return nil
	}

	// Body text starts at column 4 + 2·indent (cursor mark + indent + lead).
	textCol := m.focusLeftMargin + 4 + 2*(*body)[bodyIdx].Indent

	if press {
		m.clearFocusLink() // clicking into the body takes the caret out of the links
		m.commitBodyLine()
		raw := []rune((*body)[bodyIdx].Text)
		pos := clampInt(m.focusRowOffset[bodyRow]+msg.X-textCol, 0, len(raw))
		if bodyIdx != mod.BodyCursor {
			mod.BodyCursor = bodyIdx
			mod.BodyEditor = bodyLineEditor(string(raw))
		}
		mod.BodyEditor.SetCursor(pos)
		m.selAnchor = pos
		m.selAnchorLine = bodyIdx
		return nil
	}

	// drag — extend the selection, following the mouse across lines
	if m.selAnchor == noSelection {
		return nil
	}
	if bodyIdx != mod.BodyCursor {
		m.commitBodyLine()
		mod.BodyCursor = bodyIdx
		mod.BodyEditor = bodyLineEditor((*body)[bodyIdx].Text) // not newBodyEditor — the anchor must survive
	}
	runes := []rune(mod.BodyEditor.Value())
	mod.BodyEditor.SetCursor(clampInt(m.focusRowOffset[bodyRow]+msg.X-textCol, 0, len(runes)))
	return m.copyBodySelection()
}

func (m *Model) renderFocusContent() string {
	mod := m.modal
	m.focusRowLine = m.focusRowLine[:0]
	m.focusRowOffset = m.focusRowOffset[:0]
	m.focusCaretLine = 0
	m.focusCodeSpans = nil
	m.focusLinks = nil
	m.focusBodyLineStart = 0

	var b strings.Builder
	ln := 0 // lines emitted so far, so we can record where the caret lands
	emit := func(s string) {
		b.WriteString(s)
		b.WriteString("\n")
		ln++
	}

	switch mod.Kind {
	case ModalQuestDetail:
		q := m.findQuest(mod.QuestID)
		if q == nil {
			return ""
		}
		glyph, glyphStyle := ui.QuestGlyph(q)
		title := q.Title
		if title == "" {
			title = "Untitled quest"
		}
		emit(glyphStyle.Render(glyph) + " " + ui.StyleTitle.Render(title))
		emit("")
		// Integration codes stacked vertically under the title, then a blank
		// gap before the body (see the design). Clickable spans are recorded
		// against their content-line index for handleFocusMouse.
		if m.integrationsEnabled {
			codeLines := m.focusCodeLines(q, ln)
			for _, line := range codeLines {
				emit(line)
			}
			if len(codeLines) > 0 {
				emit("")
			}
		}
		m.focusBodyLineStart = ln
		for i, l := range q.Body {
			// While the link cursor owns the caret, the body line isn't the
			// caret line — focusCodeLines already recorded the focused link's
			// line, so don't let the body's own cursor overwrite it.
			rows, caret := m.renderBodyLineWrapped(i, l, !m.onFocusLink() && i == mod.BodyCursor, m.focusTextWidth, ln)
			for ri, row := range rows {
				if !m.onFocusLink() && ri == caret {
					m.focusCaretLine = ln
				}
				emit(row)
			}
		}
		return strings.TrimRight(b.String(), "\n")

	case ModalCampaignDetail:
		p := m.findProject(mod.CampaignID)
		if p == nil {
			return ""
		}
		name := p.Name
		if name == "" {
			name = "Untitled campaign"
		}
		done, total := ui.ProjectProgress(m.store, p.ID)
		progress := ui.StyleMuted.Render(fmt.Sprintf(" %s %d/%d", model.ProgressBucket(done, total), done, total))

		emit(ui.StyleTitle.Render(name) + progress)
		emit("")
		m.focusBodyLineStart = ln
		for i, l := range p.Body {
			editing := !mod.InQuestList && i == mod.BodyCursor
			rows, caret := m.renderBodyLineWrapped(i, l, editing, m.focusTextWidth, ln)
			for ri, row := range rows {
				if ri == caret {
					m.focusCaretLine = ln
				}
				emit(row)
			}
		}

		emit("")
		emit(ui.StyleSectionHeader.Render("Quests"))
		for _, row := range campaignQuestRows(m.store, p.ID) {
			isCursor := mod.InQuestList && m.cursor.matches(row)
			confirming := isCursor && m.confirmDeleteID != "" && rowMatchesConfirmDelete(row, m.confirmDeleteID)
			warning := m.warningText != "" && m.warningTarget.matches(row)
			titleView := ""
			if warning {
				titleView = ui.StyleMuted.Render(m.warningText)
			} else if isCursor && m.editor != nil {
				titleView = m.renderEditableStyled(m.editor, m.cursorTitleStyle(row))
			}
			hint := ""
			if confirming {
				hint = "  " + ui.StyleImportant.Render(m.confirmDeleteHint(row))
			} else if !warning {
				hint = m.actionHint(row, isCursor)
			}
			line, _ := ui.RenderRow(row, m.store, titleView, isCursor, 80, hint)
			if isCursor {
				m.focusCaretLine = ln
			}
			emit(line)
		}
		return strings.TrimRight(b.String(), "\n")

	case ModalSectionDetail:
		emit(ui.StyleTitle.Render(m.sectionTitle(mod.Section)))
		emit("")
		rows := sectionRows(m.store, mod.Section)
		if len(rows) == 0 {
			emit(ui.StyleMuted.Render("  (empty)"))
			return strings.TrimRight(b.String(), "\n")
		}
		for _, row := range rows {
			isCursor := m.cursor.matches(row)
			confirming := isCursor && m.confirmDeleteID != "" && rowMatchesConfirmDelete(row, m.confirmDeleteID)
			warning := m.warningText != "" && m.warningTarget.matches(row)
			titleView := ""
			if warning {
				titleView = ui.StyleMuted.Render(m.warningText)
			} else if isCursor && m.editor != nil {
				titleView = m.renderEditableStyled(m.editor, m.cursorTitleStyle(row))
			}
			hint := ""
			if confirming {
				hint = "  " + ui.StyleImportant.Render(m.confirmDeleteHint(row))
			} else if !warning {
				hint = m.actionHint(row, isCursor)
			}
			line, _ := ui.RenderRow(row, m.store, titleView, isCursor, 80, hint)
			if isCursor {
				m.focusCaretLine = ln
			}
			emit(line)
		}
		return strings.TrimRight(b.String(), "\n")
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
