package app

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

// isPickerModal reports whether kind is one of the small filtered-list dialogs
// that support click-to-select and wheel-to-scroll (see handlePickerClick).
func isPickerModal(k ModalKind) bool {
	return k == ModalProjectPicker || k == ModalAgentPicker
}

// handlePickerClick resolves a left-click on a picker list item to its index,
// highlights it, and confirms it exactly as pressing Enter would — reusing
// updateModal so each picker's confirm logic (move / pin / attach) isn't
// duplicated here.
func (m *Model) handlePickerClick(msg tea.MouseClickMsg) tea.Cmd {
	if m.modal == nil || m.modalItemTop < 0 {
		return nil
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	if mouse.X < m.modalItemX0 || mouse.X >= m.modalItemX1 {
		return nil
	}
	idx := mouse.Y - m.modalItemTop
	if idx < 0 || idx >= m.modalItemCount {
		return nil
	}
	m.modal.PickerIndex = idx
	return m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// handlePickerWheel moves the picker's highlight up/down with the scroll wheel.
func (m *Model) handlePickerWheel(msg tea.MouseWheelMsg) tea.Cmd {
	if m.modal == nil {
		return nil
	}
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		return m.updateModal(tea.KeyPressMsg{Code: tea.KeyUp})
	case tea.MouseWheelDown:
		return m.updateModal(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	return nil
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
// sectionRows is the navigable row list for a section's focused page — the
// same content the Tavern box shows (see ui.SectionContent), so both views
// render identically; the focused page just has more vertical room.
func (m *Model) sectionRows(section string) []ui.Row {
	return ui.SectionContent(m.store, section, m.collapsedProjects)
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

// clipLabel truncates a picker label to at most max display columns (with a
// trailing ellipsis), so every item stays on a single line — long agent titles
// would otherwise wrap and both look messy and throw off the click-to-item
// mapping (which assumes one screen row per item).
func clipLabel(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 1 {
		return ""
	}
	return string(r[:max-1]) + "…"
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
	case "runes":
		return fmt.Sprintf("Runes (%d)", ui.CountRunes(m.store))
	case "campaigns":
		return "Campaigns"
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

// moveBodyLine swaps the current body line with its neighbor above (delta<0) or
// below (delta>0), keeping the cursor on the moved line — the editor's
// Alt+↑/↓ "move line" shortcut. A no-op at the top/bottom edge.
func (m *Model) moveBodyLine(delta int) {
	mod := m.modal
	body := m.currentBody()
	if body == nil {
		return
	}
	m.commitBodyLine() // fold the in-progress edit into the line before swapping
	i := mod.BodyCursor
	j := i + delta
	if i < 0 || i >= len(*body) || j < 0 || j >= len(*body) {
		return
	}
	(*body)[i], (*body)[j] = (*body)[j], (*body)[i]
	mod.BodyCursor = j
	m.touchBodyOwner()
	m.seedBodyEditor(j, len([]rune((*body)[j].Text)))
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

// bodyCaretAtStart reports whether the body editor's caret is at the start of
// its line — the condition for ← to cross into the Sigils pane.
func (m *Model) bodyCaretAtStart() bool {
	mod := m.modal
	if mod == nil {
		return false
	}
	return mod.BodyEditor.Position() == 0
}

// handleBodyOutlineKey handles the body-outline editing keys shared by
// ModalQuestDetail and ModalCampaignDetail — the line split/merge/exit
// behaviors of a normal multiline editor (modeled on Obsidian/Notion list
// editing), plus Ctrl+D objective toggling and multiline paste.
// handled=false means the caller should forward msg to the line editor as
// ordinary text input.
func (m *Model) handleBodyOutlineKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	mod := m.modal
	body := m.currentBody()
	if body == nil {
		return nil, false
	}

	switch {
	case msg.String() == "alt+up":
		m.moveBodyLine(-1)
		return nil, true
	case msg.String() == "alt+down":
		m.moveBodyLine(1)
		return nil, true

	case msg.Code == tea.KeyEnter:
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

	case msg.Code == tea.KeyBackspace:
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

	case msg.Code == tea.KeyDelete:
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

	case msg.Code == tea.KeyTab && msg.Mod&tea.ModShift != 0:
		m.indentBodyLine(-1)
		return nil, true

	case msg.Code == tea.KeyTab:
		m.indentBodyLine(1)
		return nil, true

	case msg.String() == "ctrl+d":
		m.commitBodyLine()
		body = m.currentBody()
		burstX := m.cursorScreenX + bodyObjCol + 2*(*body)[mod.BodyCursor].Indent
		cmd := m.toggleBodyObjective(mod.BodyCursor, burstX, m.cursorScreenY)
		mod.BodyEditor = m.newBodyEditor((*body)[mod.BodyCursor].Text)
		return cmd, true
	}

	return nil, false
}

// toggleBodyObjective checks/unchecks the objective at body line idx (a no-op
// on a non-objective line), firing the same check-off burst + sound as the
// Wilds when it becomes done, at (burstX, burstY). Shared by Ctrl+D (cursor
// cell) and a checkbox click (clicked cell) — one definition so both behave
// identically (see docs/ui-consistency.md).
func (m *Model) toggleBodyObjective(idx, burstX, burstY int) tea.Cmd {
	body := m.currentBody()
	if body == nil || idx < 0 || idx >= len(*body) {
		return nil
	}
	if kind, _ := model.ClassifyBodyLine((*body)[idx].Text); kind != model.BodyObjective {
		return nil
	}
	done := !(*body)[idx].Done
	(*body)[idx].Done = done
	m.touchBodyOwner()
	if !done {
		return nil
	}
	m.spawnSparkleBurst(burstX, burstY, 10)
	return tea.Batch(m.playSound(sndObjectiveDone), m.maybeStartOverlayTick())
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

// pasteIntoModal routes a bracketed-paste (tea.PasteMsg, decoupled from key
// events in v2 — unlike v1, where a multi-line paste arrived as a multi-rune
// KeyMsg) to whichever field the open modal is actually editing.
func (m *Model) pasteIntoModal(msg tea.PasteMsg) tea.Cmd {
	mod := m.modal
	// A paste while renaming a title goes into the title editor, not the body.
	if m.titleEditor != nil && isFocusModal(mod.Kind) {
		var cmd tea.Cmd
		*m.titleEditor, cmd = m.titleEditor.Update(msg)
		return cmd
	}
	// Likewise for an inline Lookout rename.
	if m.lookoutEditor != nil && mod.Kind == ModalQuestDetail {
		var cmd tea.Cmd
		*m.lookoutEditor, cmd = m.lookoutEditor.Update(msg)
		return cmd
	}
	switch mod.Kind {
	case ModalQuestDetail:
		start, end := m.pasteBodyLines(msg.Content)
		if q := m.findQuest(mod.QuestID); q != nil {
			return tea.Batch(m.captureBodyLinesRange(q, start, end), m.maybeStartSpinner())
		}
		return nil
	case ModalCampaignDetail:
		if mod.InQuestList {
			if m.editor == nil {
				return nil
			}
			var cmd tea.Cmd
			*m.editor, cmd = m.editor.Update(msg)
			return cmd
		}
		m.pasteBodyLines(msg.Content)
		return nil
	case ModalSectionDetail:
		if m.editor == nil {
			return nil
		}
		var cmd tea.Cmd
		*m.editor, cmd = m.editor.Update(msg)
		return cmd
	case ModalProjectPicker, ModalAgentPicker:
		mod.PickerFilter += msg.Content
		mod.PickerIndex = 0
		return nil
	}
	return nil
}

// focusScrollBy moves the focus-view caret n rows up/down by replaying that
// many Up/Down key presses through updateModal — so wheel and Page keys
// reuse the exact navigation logic (body rows, quest-list transitions), and
// the view's caret-driven scroll follows along.
func (m *Model) focusScrollBy(down bool, n int) tea.Cmd {
	k := tea.KeyPressMsg{Code: tea.KeyUp}
	if down {
		k.Code = tea.KeyDown
	}
	var cmd tea.Cmd
	for i := 0; i < n; i++ {
		cmd = m.updateModal(k)
	}
	return cmd
}

// beginTitleEdit opens an inline editor over a detail page's title so it can
// be renamed in place. Only quests and campaigns have editable titles; section
// pages are fixed and ignore it.
func (m *Model) beginTitleEdit() {
	mod := m.modal
	if mod == nil {
		return
	}
	var cur string
	switch mod.Kind {
	case ModalQuestDetail:
		q := m.findQuest(mod.QuestID)
		if q == nil {
			return
		}
		cur = q.Title
	case ModalCampaignDetail:
		p := m.findProject(mod.CampaignID)
		if p == nil {
			return
		}
		cur = p.Name
	default:
		return
	}
	m.titleEditFromSigils = m.onFocusLink() // so Down returns to the right pane
	m.commitBodyLine()                      // persist any in-flight body edit before switching focus
	m.clearFocusLink()                      // leave whichever pane owned the caret
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(cur)
	ti.CursorEnd()
	_ = ti.Focus()
	m.titleEditor = &ti
	m.clearSelection()
	m.invalidateRender()
}

// commitTitleEdit writes the inline title editor back to the quest/campaign
// (ignoring an all-whitespace value so a title can't be blanked out) and closes
// the editor.
func (m *Model) commitTitleEdit() {
	if m.titleEditor == nil {
		return
	}
	value := strings.TrimSpace(m.titleEditor.Value())
	if mod := m.modal; mod != nil && value != "" {
		switch mod.Kind {
		case ModalQuestDetail:
			if q := m.findQuest(mod.QuestID); q != nil {
				q.Title = value
				q.UpdatedAt = time.Now()
			}
		case ModalCampaignDetail:
			if p := m.findProject(mod.CampaignID); p != nil {
				p.Name = value
			}
		}
		m.save()
	}
	m.titleEditor = nil
	m.clearSelection()
	m.invalidateRender()
}

// cancelTitleEdit closes the inline title editor without saving.
func (m *Model) cancelTitleEdit() {
	m.titleEditor = nil
	m.clearSelection()
	m.invalidateRender()
}

// returnFromTitleEdit hands focus back to whichever pane the title editor was
// entered from — the top of Sigils, or the top of the body.
func (m *Model) returnFromTitleEdit() {
	if m.titleEditFromSigils && m.integrationsEnabled {
		m.focusLinkIdx = 0
		return
	}
	m.clearFocusLink()
	if mod := m.modal; mod != nil {
		mod.InQuestList = false
	}
	if body := m.currentBody(); body != nil && len(*body) > 0 {
		m.seedBodyEditor(0, 0)
	}
}

func (m *Model) updateModal(msg tea.KeyPressMsg) tea.Cmd {
	mod := m.modal

	// An inline title rename owns every key while it's open.
	if m.titleEditor != nil && isFocusModal(mod.Kind) {
		switch msg.Code {
		case tea.KeyEsc:
			m.cancelTitleEdit()
			return nil
		case tea.KeyEnter:
			m.commitTitleEdit()
			return nil
		case tea.KeyDown:
			// Down commits and returns to the top of the pane it came from.
			m.commitTitleEdit()
			m.returnFromTitleEdit()
			return nil
		}
		if handled, cmd := m.applySelectionKey(m.titleEditor, msg); handled {
			return cmd
		}
		var cmd tea.Cmd
		*m.titleEditor, cmd = m.titleEditor.Update(msg)
		return cmd
	}
	// F2 starts an inline rename of the current detail page's title.
	if isFocusModal(mod.Kind) && key.Matches(msg, Keys.Rename) {
		m.beginTitleEdit()
		return nil
	}

	// PageUp/PageDown scroll a focused quest/campaign by half a screen.
	if isFocusModal(mod.Kind) && (msg.Code == tea.KeyPgUp || msg.Code == tea.KeyPgDown) {
		half := m.height / 2
		if half < 1 {
			half = 1
		}
		return m.focusScrollBy(msg.Code == tea.KeyPgDown, half)
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
			if msg.Text != "" {
				mod.PickerFilter += msg.Text
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
					id := items[mod.PickerIndex].ID
					if indexOfStr(target.AgentWorkspaces, id) < 0 {
						target.AgentWorkspaces = append(target.AgentWorkspaces, id)
					}
					target.UpdatedAt = time.Now()
					m.save()
					// Reflect the pinned agent immediately, and make sure the
					// poll is running now that a workspace is pinned.
					m.pendingConnBurstCode = id
					cmd = tea.Batch(refreshAgentsCmd(), m.maybeStartAgentPoll(), m.playSound(sndAddConnection), m.pokeOverlayTick())
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
			if msg.Text != "" {
				mod.PickerFilter += msg.Text
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
		// An inline Lookout rename owns every key while it's open (shared logic).
		if cmd, handled := m.handleLookoutRenameKey(msg); handled {
			return cmd
		}
		// Alt+←/→ resize the Sigils/body split (the keyboard twin of dragging the
		// divider); →/↓ grow the Sigils pane, ←/↑ shrink it. Alt+↑/↓ only resize
		// while a Sigil is focused (in the body they stay move-line) — and many
		// macOS terminals only deliver Option+↑/↓ as alt-arrows (Option+←/→ become
		// word-motion), so accepting ↑/↓ here guarantees a working key in Sigils.
		switch msg.String() {
		case "alt+right":
			m.resizeColumnWidth(resizeStep)
			return nil
		case "alt+left":
			m.resizeColumnWidth(-resizeStep)
			return nil
		case "alt+down":
			if m.onFocusLink() {
				m.resizeColumnWidth(resizeStep)
				return nil
			}
		case "alt+up":
			if m.onFocusLink() {
				m.resizeColumnWidth(-resizeStep)
				return nil
			}
		}
		m.cursorMoved = true // a keypress re-centers the active pane on its caret
		// Ctrl+1 / Ctrl+2 jump between the two panes (Sigils / body).
		if m.integrationsEnabled && m.focusLinkCount(q) > 0 {
			switch msg.String() {
			case "ctrl+1":
				m.commitBodyLine()
				m.focusLinkIdx = 0
				return nil
			case "ctrl+2":
				if m.onFocusLink() {
					m.clearFocusLink()
					m.seedBodyEditor(mod.BodyCursor, 0)
				}
				return nil
			}
		}
		// The link cursor owns navigation while a Sigils link is focused — Enter
		// opens, Ctrl+X arms removal, up/down step through the links, → returns
		// to the body.
		if m.onFocusLink() {
			if cmd, handled := m.handleFocusLinkKey(msg, q); handled {
				return cmd
			}
		}
		if key.Matches(msg, Keys.Find) {
			return m.findTracksInTrails(q.ID)
		}
		if msg.Code == tea.KeyEsc {
			m.commitBodyLine()
			m.closeModal()
			return nil
		}
		if handled, cmd := m.applyBodySelectionKey(msg); handled {
			return cmd
		}
		// ← at the start of a body line crosses left into the Sigils pane.
		if msg.Code == tea.KeyLeft && m.bodyCaretAtStart() && m.integrationsEnabled && m.focusLinkCount(q) > 0 {
			m.commitBodyLine()
			m.focusLinkIdx = 0
			return nil
		}
		if msg.String() == "up" {
			// At the top visual row, Up jumps to the title for renaming.
			if !m.moveBodyCursor(-1) {
				m.beginTitleEdit()
			}
			return nil
		}
		if msg.String() == "down" {
			m.moveBodyCursor(1)
			return nil
		}
		if cmd, handled := m.handleBodyOutlineKey(msg); handled {
			// A multiline paste may have captured links (now fetching) — make
			// sure the spinner is running to animate them.
			return tea.Batch(cmd, m.maybeStartSpinner())
		}
		var cmd tea.Cmd
		mod.BodyEditor, cmd = mod.BodyEditor.Update(msg)
		// After an ordinary edit, if the current line now holds a complete
		// Jira/PR URL, capture it, shorten it inline, and fire an immediate sync
		// for just the new code(s) — animating the "fetching" state meanwhile.
		if syncCmd := m.captureCurrentBodyLink(q); syncCmd != nil {
			return tea.Batch(cmd, syncCmd, m.maybeStartSpinner())
		}
		return cmd

	case ModalCampaignDetail:
		p := m.findProject(mod.CampaignID)
		if p == nil {
			m.closeModal()
			return nil
		}
		if msg.Code == tea.KeyEsc {
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
			// At the top visual row, Up jumps to the title for renaming.
			if !m.moveBodyCursor(-1) {
				m.beginTitleEdit()
			}
			return nil
		}
		if cmd, handled := m.handleBodyOutlineKey(msg); handled {
			return cmd
		}
		var cmd tea.Cmd
		mod.BodyEditor, cmd = mod.BodyEditor.Update(msg)
		return cmd

	case ModalSectionDetail:
		if msg.Code == tea.KeyEsc {
			m.commitEdit()
			m.setCursor(ui.Row{Kind: ui.RowSection, Section: mod.Section})
			m.closeModal()
			return nil
		}
		rows := m.sectionRows(mod.Section)
		switch msg.String() {
		case "up":
			if r, ok := stepSelectable(rows, m.cursor, -1); ok {
				m.commitEdit()
				m.setCursor(r)
			}
			return nil
		case "down":
			if r, ok := stepSelectable(rows, m.cursor, 1); ok {
				m.commitEdit()
				m.setCursor(r)
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

	// Picker list geometry, filled in by the picker cases below and turned into
	// screen coordinates once the box is placed (see the tail of this function).
	m.modalItemTop, m.modalItemCount = -1, 0
	pickerFirstLine, pickerItemCount := -1, 0

	switch mod.Kind {
	case ModalHelp:
		var b strings.Builder
		b.WriteString(ui.StyleTitle.Render("Quests"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("A single fluid outline for tracking quests inside campaigns."))
		b.WriteString("\n\n")

		b.WriteString(ui.StyleSectionHeader.Render("Views"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-11s%s\n", "Tavern", ui.StyleMuted.Render("the full outline — everything at once"))
		fmt.Fprintf(&b, "%-11s%s\n", "Wilds", ui.StyleMuted.Render("Ctrl+G — a focused view of just your taken-up quests"))
		b.WriteString("\n")

		b.WriteString(ui.StyleSectionHeader.Render("Sections"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("Ctrl+1–5 jump to one; Ctrl+Shift+1–4 collapse a rail section."))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("Alt+←/→ resize the rail width; Alt+↑/↓ resize the focused box's height."))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-9s%-13s%s\n", "Ctrl+1", "Questboard", ui.StyleMuted.Render("your inbox — new quests with no campaign yet"))
		fmt.Fprintf(&b, "%-9s%-13s%s\n", "Ctrl+2", "Runes", ui.StyleMuted.Render("feature flags you're watching, grouped by quest"))
		fmt.Fprintf(&b, "%-9s%-13s%s\n", "Ctrl+3", "Lookouts", ui.StyleMuted.Render("usage dashboards you're monitoring, grouped by quest"))
		fmt.Fprintf(&b, "%-9s%-13s%s\n", "Ctrl+4", "Vault", ui.StyleMuted.Render("your archive — parked quests and retired campaigns"))
		fmt.Fprintf(&b, "%-9s%-13s%s\n", "Ctrl+5", "Campaigns", ui.StyleMuted.Render("your projects — each lists its own quests"))
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
		b.WriteString(ui.StyleMuted.Render("A quest links to other systems, each a status-colored emblem by its title:"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-14s%s\n", "Jira", ui.StyleMuted.Render("paste an issue URL into the quest body"))
		fmt.Fprintf(&b, "%-14s%s\n", "GitHub PR", ui.StyleMuted.Render("paste a PR URL into the quest body"))
		fmt.Fprintf(&b, "%-14s%s\n", "LaunchDarkly", ui.StyleMuted.Render("watch a flag — in the Runes section or a quest's detail"))
		fmt.Fprintf(&b, "%-14s%s\n", "herdr", ui.StyleMuted.Render("pin a Claude agent from a quest's detail view"))
		fmt.Fprintf(&b, "%-14s%s\n", "Analytics", ui.StyleMuted.Render("find Tracks in a quest's PRs; paste a dashboard URL"))
		fmt.Fprintf(&b, "%-14s%s\n", "", ui.StyleMuted.Render("as a Lookout — monitored here in the Lookouts section"))
		b.WriteString(ui.StyleMuted.Render("Jira/PR live status needs gh and acli logged in locally."))
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
		pickerFirstLine, pickerItemCount = strings.Count(b.String(), "\n"), len(items)
		for i, item := range items {
			label := clipLabel(item.Label, 54)
			line := "  " + label
			if i == mod.PickerIndex {
				line = ui.StyleSelectedRow.Render("> " + label)
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
			b.WriteString(ui.StyleMuted.Render("  (no herdr agents — is the herdr server running?)") + "\n")
		}
		pickerFirstLine, pickerItemCount = strings.Count(b.String(), "\n"), len(items)
		for i, item := range items {
			// herdr-style row: a status icon in a left gutter (kept outside the
			// selection highlight so its color survives), then "<workspace> ·
			// <tab>" — workspace emphasized, tab muted. Look the agent up by its
			// pinned terminal id for the live status/labels.
			ag, _ := m.matchAgent(item.ID)
			icon := m.agentGlyph(ag.Status)
			ws, tab := ag.Workspace, ag.Tab
			var body string
			if i == mod.PickerIndex {
				body = ui.StyleSelectedRow.Render(clipLabel(item.Label, 54))
			} else if tab == "" {
				body = ui.StyleName.Render(clipLabel(ws, 54))
			} else {
				body = ui.StyleName.Render(ws) + ui.StyleMuted.Render(" · "+tab)
			}
			b.WriteString("  " + icon + " " + body + "\n")
		}
		b.WriteString("\n" + ui.StyleMuted.Render("type to filter · ↑↓ choose · enter pin · esc cancel"))
		content = b.String()

	case ModalDetailHelp:
		var b strings.Builder
		b.WriteString(ui.StyleTitle.Render("Quest & campaign details"))
		b.WriteString("\n\n")

		b.WriteString(ui.StyleSectionHeader.Render("Title & body"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-12s%s\n", "F2", ui.StyleMuted.Render("rename the title (or click it); Enter saves, Esc cancels"))
		fmt.Fprintf(&b, "%-12s%s\n", "Alt+←/→", ui.StyleMuted.Render("resize the Sigils / body split (Alt+↑/↓ too when in Sigils)"))
		fmt.Fprintf(&b, "%-12s%s\n", `# `, ui.StyleMuted.Render("start a line with this for a heading"))
		fmt.Fprintf(&b, "%-12s%s\n", `- `, ui.StyleMuted.Render("start an objective; Ctrl+D checks it off"))
		b.WriteString("\n")

		b.WriteString(ui.StyleSectionHeader.Render("Integrations"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("Paste a Jira/PR URL into the body — captured and shortened to a code:"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-14s%s\n", "GitHub PR", ui.StyleMuted.Render("CI + review state, unresolved comments, merged;"))
		fmt.Fprintf(&b, "%-14s%s\n", "", ui.StyleMuted.Render("several linked PRs order into a Graphite stack"))
		fmt.Fprintf(&b, "%-14s%s\n", "Jira", ui.StyleMuted.Render("issue status — todo / in progress / done"))
		fmt.Fprintf(&b, "%-14s%s\n", "LaunchDarkly", ui.StyleMuted.Render("Runes — flag on/off state, found from LD links in PR bodies"))
		fmt.Fprintf(&b, "%-14s%s\n", "herdr", ui.StyleMuted.Render("\"+ add agent\" pins a Claude agent; Enter jumps to it"))
		fmt.Fprintf(&b, "%-14s%s\n", "Find", ui.StyleMuted.Render("Ctrl+R (or the affordance) finds Tracks + flags/issues in"))
		fmt.Fprintf(&b, "%-14s%s\n", "", ui.StyleMuted.Render("the quest's Trails; also runs on sync when a PR changes"))
		fmt.Fprintf(&b, "%-14s%s\n", "Track", ui.StyleMuted.Render("a found event (green = live in prod, amber = pending);"))
		fmt.Fprintf(&b, "%-14s%s\n", "", ui.StyleMuted.Render("Ctrl+X dismisses it, \"N dismissed\" restores by mistake"))
		fmt.Fprintf(&b, "%-14s%s\n", "Lookout", ui.StyleMuted.Render("paste a dashboard URL to monitor usage; Enter scries, r renames"))
		fmt.Fprintf(&b, "%-14s%s\n", "Plans", ui.StyleMuted.Render("\"write the Lookout's plans\" (i) → a dashboard-build"))
		fmt.Fprintf(&b, "%-14s%s\n", "", ui.StyleMuted.Render("prompt covering all the quest's Tracks, copied to paste"))
		b.WriteString(ui.StyleMuted.Render("On a link: ↑/↓ focus, c copies (a section header copies the whole"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("list to share), Enter opens, Ctrl+X removes. Click a link to copy,"))
		b.WriteString("\n")
		b.WriteString(ui.StyleMuted.Render("double-click to open. Pasted links shorten inline. Needs gh + acli."))
		b.WriteString("\n\n")

		b.WriteString(ui.StyleSectionHeader.Render("Quest list (in a campaign)"))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%-12s%s\n", "Tab", ui.StyleMuted.Render("open a listed quest's own detail"))
		fmt.Fprintf(&b, "%-12s%s\n", "Enter", ui.StyleMuted.Render("add via \"+ New Quest\", or a sibling below one"))
		fmt.Fprintf(&b, "%-12s%s\n", "Ctrl+D/G/T", ui.StyleMuted.Render("toggle done / taken / type"))
		fmt.Fprintf(&b, "%-12s%s\n", "Ctrl+P", ui.StyleMuted.Render("move to another campaign"))
		fmt.Fprintf(&b, "%-12s%s\n", "Ctrl+X", ui.StyleMuted.Render("delete (inline y/n)"))
		fmt.Fprintf(&b, "%-12s%s\n", "Shift+↑/↓", ui.StyleMuted.Render("reorder"))
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

	// Map the picker's list items to screen coordinates now that the box's size
	// is known — lipgloss.Place centers it, so item i sits at boxTop + border(1)
	// + padding(1) + its content-line index. Lets a click resolve to an item.
	if pickerFirstLine >= 0 && pickerItemCount > 0 {
		boxLines := strings.Split(box, "\n")
		boxTop := (m.height - len(boxLines)) / 2
		if boxTop < 0 {
			boxTop = 0
		}
		boxLeft := (m.width - lipgloss.Width(boxLines[0])) / 2
		if boxLeft < 0 {
			boxLeft = 0
		}
		m.modalItemTop = boxTop + 2 + pickerFirstLine
		m.modalItemCount = pickerItemCount
		m.modalItemX0 = boxLeft
		m.modalItemX1 = boxLeft + lipgloss.Width(boxLines[0])
	}

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// renderFocusView takes over the whole screen with just the quest or
// campaign under Tab — not a boxed dialog, a focused view of that one
// thing. "← back (esc)" is the only way out; every other interaction
// (editing, adding/removing quests, toggling done, etc.) works exactly as
// it does in the main outline.
// viewQuestDetail renders the two-pane quest detail: a bordered, independently
// scrollable "Sigils" box on the left and the borderless, independently
// scrollable body on the right, split by a draggable divider (the box's right
// border). Reuses drawBox + scrollWindow + scrollWithMargin; the divider drag
// reuses the resizeDrag machinery (see updateResizeDrag/endResizeDrag).
func (m *Model) viewQuestDetail() string {
	mod := m.modal
	q := m.findQuest(mod.QuestID)
	if q == nil {
		return ""
	}

	contentWidth := clampInt(m.width-8, 20, 150)
	leftMargin := (m.width - contentWidth) / 2
	if leftMargin < 0 {
		leftMargin = 0
	}
	margin := strings.Repeat(" ", leftMargin)

	// Header (back / help), clickable.
	back := ui.StyleMuted.Render("← back (esc)")
	right := ui.StyleMuted.Render("F1 help")
	if m.clipboardToastActive {
		right = renderClipboardToast(m.clipboardToastText)
	}
	hpad := contentWidth - lipgloss.Width(back) - lipgloss.Width(right)
	if hpad < 1 {
		hpad = 1
	}
	headerLine := back + strings.Repeat(" ", hpad) + right
	m.focusLeftMargin = leftMargin // the back-click hit-test (handleFocusPointer) reads this
	m.focusBackWidth = lipgloss.Width(back)
	m.focusHelpX = leftMargin + m.focusBackWidth + hpad
	m.focusHelpWidth = lipgloss.Width(right)
	if m.clipboardToastActive {
		m.focusHelpWidth = 0
	}

	// Title + type/status/progress chip.
	glyph, glyphStyle := ui.QuestGlyph(q)
	title := q.Title
	if title == "" {
		title = "Untitled quest"
	}
	chip := "  " + questTypeLabel(q) + " · " + m.questStatusLabel(q)
	if done, total := q.ObjectiveProgress(); total > 0 {
		chip += fmt.Sprintf(" · %d/%d", done, total)
	}
	// A 2-col cursor-mark slot (like every outline row) leads the title; it
	// shows the accent "› " while the title is being renamed, blank otherwise.
	mark := "  "
	if m.titleEditor != nil {
		mark = ui.StyleCursor.Render(ui.GlyphCursor)
	}
	glyphLead := glyphStyle.Render(glyph) + " "
	m.focusTitleX = leftMargin + lipgloss.Width(mark) + lipgloss.Width(glyphLead)
	m.focusTitleWidth = lipgloss.Width(ui.StyleTitle.Render(title))
	// Constant width whether renaming or not (same as the Tavern list), so the
	// chip after the title never shifts on select or as the caret moves.
	titleText := m.constantWidthTitle(title, m.titleEditor, ui.StyleTitle, ui.StyleTitle)
	// The type/status/progress chip stays visible even while renaming — the chip
	// reflects quest metadata, not the title text, so there's no reason to hide it.
	titleLine := mark + glyphLead + titleText + ui.StyleMuted.Render(chip)

	// Column geometry (draggable ratio).
	const gap = 1
	detailW := clampInt(int(float64(contentWidth)*m.detailWidthRatio+0.5), 26, contentWidth-24)
	if detailW < 20 {
		detailW = 20
	}
	bodyW := contentWidth - detailW - gap
	if bodyW < 12 {
		bodyW = 12
	}
	detailInnerW := detailW - 4
	if detailInnerW < 4 {
		detailInnerW = 4
	}
	bodyWrapW := bodyW - 4
	if bodyWrapW < 8 {
		bodyWrapW = 8
	}
	// Up/Down navigation (moveBodyCursor → bodyVisualRows → focusWrapWidth) must
	// wrap at the SAME width the body is rendered at, or the caret lands mid-line.
	m.focusTextWidth = bodyWrapW
	m.focusDetailX = leftMargin + 2           // box interior left (border + space)
	m.focusBodyX = leftMargin + detailW + gap // body column left
	m.focusBodyW = bodyW
	m.detailDividerX = leftMargin + detailW - 1 // the box's right border = divider

	// Pane height.
	vpad := viewVPad
	if maxPad := m.height / 4; vpad > maxPad {
		vpad = maxPad
	}
	if vpad < 0 {
		vpad = 0
	}
	paneHeight := m.height - 2*vpad - 5 // header + blank + title + blank + status line
	if paneHeight < 5 {
		paneHeight = 5
	}
	interiorH := paneHeight - 2 // box top + bottom border
	if interiorH < 2 {
		interiorH = 2
	}
	contentH := interiorH - 1 // reserve one interior row as top padding

	// Sigils content: focusCodeLines (spans relative to the box interior) then
	// wrap to the inner width, remapping the recorded link/span rows.
	m.focusLinks = nil
	m.focusCodeSpans = nil
	raw := m.focusCodeLines(q, 0, m.focusDetailX)
	detailLines := make([]string, 0, len(raw))
	remap := make([]int, len(raw))
	ws := lipgloss.NewStyle().Width(detailInnerW)
	for i, dl := range raw {
		remap[i] = len(detailLines)
		detailLines = append(detailLines, strings.Split(ws.Render(dl), "\n")...)
	}
	remapRow := func(r int) int {
		if r >= 0 && r < len(remap) {
			return remap[r]
		}
		return r
	}
	for i := range m.focusLinks {
		m.focusLinks[i].line = remapRow(m.focusLinks[i].line)
	}
	for i := range m.focusCodeSpans {
		m.focusCodeSpans[i].line = remapRow(m.focusCodeSpans[i].line)
	}
	detailRows := make([]int, len(detailLines))
	for i := range detailRows {
		detailRows[i] = i
	}

	// Sigils scroll: follow the focused link.
	activeRow := -1
	if m.onFocusLink() && m.focusLinkIdx >= 0 && m.focusLinkIdx < len(m.focusLinks) {
		activeRow = m.focusLinks[m.focusLinkIdx].line
	}
	maxS := len(detailLines) - contentH
	if maxS < 0 {
		maxS = 0
	}
	sScroll := m.sectionScroll["sigils"]
	if activeRow >= 0 && m.cursorMoved {
		sScroll = scrollWithMargin(activeRow, sScroll, contentH, len(detailLines))
	}
	sScroll = clampInt(sScroll, 0, maxS)
	m.sectionScroll["sigils"] = sScroll
	m.sectionMaxScroll["sigils"] = maxS
	sWin, _ := scrollWindow(detailLines, detailRows, sScroll, contentH)
	sTitle := ui.StyleMuted.Render("Sigils")
	if m.onFocusLink() {
		sTitle = ui.StyleTitle.Render("Sigils")
	}
	// Prepend a blank interior row as top padding.
	box := drawBox(sTitle, "", append([]string{""}, sWin...), detailW, lipgloss.RoundedBorder(), ui.StyleMuted)

	// Body: wrapped lines + own scroll (drop inline body-code spans — they're
	// recorded at the wrong column for this layout and are a niche feature).
	spanCut := len(m.focusCodeSpans)
	m.focusRowLine = m.focusRowLine[:0]
	m.focusRowOffset = m.focusRowOffset[:0]
	var bodyLines []string
	bodyCaretRow := 0
	for i, l := range q.Body {
		rows, caret := m.renderBodyLineWrapped(i, l, m.bodyCaretActive() && i == mod.BodyCursor, bodyWrapW, len(bodyLines))
		for ri, row := range rows {
			if m.bodyCaretActive() && ri == caret {
				bodyCaretRow = len(bodyLines)
			}
			bodyLines = append(bodyLines, row)
		}
	}
	m.focusCodeSpans = m.focusCodeSpans[:spanCut]
	bodyRows := make([]int, len(bodyLines))
	for i := range bodyRows {
		bodyRows[i] = i
	}
	maxB := len(bodyLines) - contentH
	if maxB < 0 {
		maxB = 0
	}
	bScroll := m.sectionScroll["qbody"]
	if m.bodyCaretActive() && m.cursorMoved {
		bScroll = scrollWithMargin(bodyCaretRow, bScroll, contentH, len(bodyLines))
	}
	bScroll = clampInt(bScroll, 0, maxB)
	m.sectionScroll["qbody"] = bScroll
	m.sectionMaxScroll["qbody"] = maxB
	bWin, _ := scrollWindow(bodyLines, bodyRows, bScroll, contentH)

	// Compose: box lines, with the body aligned to the box interior content
	// (box line 0 = top border, 1 = top-pad blank, 2.. = content). The 1-col gap
	// after the box's right border becomes an accent line while the divider is
	// hovered or dragged — the same "you can grab this" cue the Tavern uses.
	gapStr := strings.Repeat(" ", gap)
	if m.resizeDrag.target == resizeDetailCol || m.resizeHover == resizeDetailCol {
		gapStr = lipgloss.NewStyle().Foreground(ui.ColorAccent).Render("│")
	}
	clip := lipgloss.NewStyle().MaxWidth(m.width)
	var b strings.Builder
	for i := 0; i < vpad; i++ {
		b.WriteString("\n")
	}
	b.WriteString(clip.Render(margin+headerLine) + "\n\n")
	b.WriteString(clip.Render(margin+titleLine) + "\n\n")
	for i := 0; i < len(box); i++ {
		bodyPart := ""
		if i >= 2 && i-2 < len(bWin) {
			bodyPart = bWin[i-2]
		}
		b.WriteString(clip.Render(margin+box[i]+gapStr+bodyPart) + "\n")
	}
	// Fixed status line under the box: the focused sigil's actions — the same
	// bottom-status-line treatment every view uses (see statusHint/statusBar).
	// Always present so it never reflows the panes.
	b.WriteString(clip.Render(margin+"  "+m.statusHint()) + "\n")

	// Screen coordinates for clicks/caret/sparkle.
	m.focusHeaderRow = vpad
	m.focusTitleRow = vpad + 2 // header (vpad) + blank + title
	paneTop := vpad + 4
	m.detailPaneTop = paneTop
	m.detailPaneBottom = paneTop + len(box) - 1
	interiorTop := paneTop + 2                 // first content row (below top border + top pad)
	m.focusContentTop = interiorTop - sScroll  // detail span at content row r → interiorTop + (r - sScroll)
	m.focusBodyBaseRow = interiorTop - bScroll // body content row r → interiorTop + (r - bScroll)
	m.cursorScreenY = -1
	if m.bodyCaretActive() {
		if vis := bodyCaretRow - bScroll; vis >= 0 && vis < contentH {
			m.cursorScreenY = interiorTop + vis
			m.cursorScreenX = m.focusBodyX
		}
	}
	if m.pendingConnBurstCode != "" {
		code := m.pendingConnBurstCode
		m.pendingConnBurstCode = ""
		for _, l := range m.focusLinks {
			if l.code == code {
				if vis := l.line - sScroll; vis >= 0 && vis < contentH {
					m.spawnSparkleBurst(m.focusDetailX+2, interiorTop+vis, 10)
				}
				break
			}
		}
	}
	m.cursorMoved = false // consumed; the wheel scrolls freely until the next key move
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) renderFocusView() string {
	// Leave comfortable breathing room: at least ~4 columns each side, and
	// a blank row top and bottom (see vpad below).
	// The quest detail is two-column (details + body), so it wants more width;
	// the single-column campaign/section pages stay narrow for readability.
	maxW := 80
	if m.modal != nil && m.modal.Kind == ModalQuestDetail {
		maxW = 150
	}
	contentWidth := m.width - 8
	if contentWidth > maxW {
		contentWidth = maxW
	}
	if contentWidth < 20 {
		contentWidth = 20
	}
	leftMargin := (m.width - contentWidth) / 2
	if leftMargin < 0 {
		leftMargin = 0
	}
	margin := strings.Repeat(" ", leftMargin)
	// Defaults (single column); the quest case overrides these to place the
	// body in its right-hand column.
	m.focusDetailX = leftMargin
	m.focusBodyX = leftMargin

	back := ui.StyleMuted.Render("← back (esc)")
	right := ui.StyleMuted.Render("F1 help")
	if m.clipboardToastActive {
		right = renderClipboardToast(m.clipboardToastText)
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
		// Re-center on the caret only when it moved (keyboard); a wheel scroll
		// leaves the caret put and must stick.
		if m.cursorMoved {
			caretAbs := 2 + m.focusCaretLine
			switch {
			case caretAbs < avail:
				m.focusScroll = 0 // caret in the first screenful: show from the top (header visible)
			case caretAbs < m.focusScroll:
				m.focusScroll = caretAbs // scrolled above the window: bring it to the top edge
			case caretAbs >= m.focusScroll+avail:
				m.focusScroll = caretAbs - avail + 1 // below the window: bring it to the bottom edge
			}
		}
		m.focusScroll = clampInt(m.focusScroll, 0, len(lines)-avail)
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
	m.focusTitleRow = m.focusContentTop // campaign/section title is content line 0
	// Record the body caret's screen cell so a completion burst (Ctrl+D on an
	// objective here) fires at the checkbox — same overlay path as the outline.
	m.cursorScreenY = m.focusContentTop + m.focusCaretLine
	m.cursorScreenX = m.focusBodyX // the body caret's marker column
	// A connection was just added: burst on its own line in the Sigils section
	// (m.focusLinks was populated by renderFocusContent above).
	if m.pendingConnBurstCode != "" {
		code := m.pendingConnBurstCode
		m.pendingConnBurstCode = ""
		burstX := m.focusDetailX + 2 // the details column's connection glyph
		if burstX <= 0 {
			burstX = leftMargin + 6
		}
		for _, l := range m.focusLinks {
			if l.code == code {
				m.spawnSparkleBurst(burstX, m.focusContentTop+l.line, 10)
				break
			}
		}
	}
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
	// Fixed bottom status line — the cursor row's actions, same as every other
	// view (Tavern, Wilds, quest detail).
	if status := m.statusHint(); status != "" {
		b.WriteString(clip.Render(margin+strings.TrimPrefix(status, "  ")) + "\n")
	}
	m.cursorMoved = false // consumed; the wheel scrolls freely until the next key move
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
// handleFocusWheel is the focus view's scroll-wheel handling — v1 folded this
// into handleFocusMouse guarded by Action==Press; v2 gives wheel events their
// own message type, dispatched separately from Update.
func (m *Model) handleFocusWheel(msg tea.MouseWheelMsg) tea.Cmd {
	mouse := msg.Mouse()
	delta := -1
	if mouse.Button == tea.MouseWheelDown {
		delta = 1
	}
	// The wheel scrolls the VIEWPORT only — never the cursor/caret (same rule as
	// every other list). The quest detail has two independently-scrolled panes;
	// the wheel scrolls whichever the pointer is over. The section/campaign focus
	// page has one scroll offset.
	if m.modal != nil && m.modal.Kind == ModalQuestDetail {
		section := "qbody"
		if mouse.X < m.focusBodyX {
			section = "sigils"
		}
		next := m.sectionScroll[section] + delta
		if next < 0 {
			next = 0
		}
		if max := m.sectionMaxScroll[section]; next > max {
			next = max
		}
		m.sectionScroll[section] = next
		m.invalidateRender()
		return nil
	}
	m.focusScroll += delta // section/campaign page viewport; renderFocusView clamps
	m.invalidateRender()
	return nil
}

// handleFocusClick is a left mouse press inside a focus view.
func (m *Model) handleFocusClick(msg tea.MouseClickMsg) tea.Cmd {
	if m.modal == nil {
		return nil
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	// Grab the Sigils box's right border (or the accent gap beside it) to drag
	// the pane divider.
	if m.overDetailDivider(mouse) {
		m.resizeDrag = resizeDragState{active: true, target: resizeDetailCol}
		return nil
	}
	return m.handleFocusPointer(mouse, true)
}

// overDetailDivider reports whether the pointer is on the quest-detail divider
// grab target (the box's right border or the gap column beside it).
func (m *Model) overDetailDivider(mouse tea.Mouse) bool {
	return m.modal != nil && m.modal.Kind == ModalQuestDetail &&
		(mouse.X == m.detailDividerX || mouse.X == m.detailDividerX+1) &&
		mouse.Y >= m.detailPaneTop && mouse.Y <= m.detailPaneBottom
}

// handleFocusMotion is mouse movement inside a focus view — a divider drag, a
// divider-hover highlight, or a drag extending the body selection (m.selAnchor).
func (m *Model) handleFocusMotion(msg tea.MouseMotionMsg) tea.Cmd {
	if m.modal == nil {
		return nil
	}
	mouse := msg.Mouse()
	if m.resizeDrag.active && m.resizeDrag.target == resizeDetailCol {
		m.updateResizeDrag(mouse.X, mouse.Y)
		return nil
	}
	// Divider hover highlight (quest detail).
	if m.modal.Kind == ModalQuestDetail {
		want := resizeNone
		if m.overDetailDivider(mouse) {
			want = resizeDetailCol
		}
		if m.resizeHover != want {
			m.resizeHover = want
			m.invalidateRender()
		}
	}
	return m.handleFocusPointer(mouse, false)
}

// handleFocusPointer is the shared logic behind handleFocusClick/
// handleFocusMotion — v1 combined both into one handleFocusMouse keyed off
// msg.Action; the `press` flag now plays that role explicitly since v2 splits
// press and motion into distinct message types.
func (m *Model) handleFocusPointer(mouse tea.Mouse, press bool) tea.Cmd {
	mod := m.modal

	if press && mouse.Y == m.focusHeaderRow {
		switch {
		case mouse.X >= m.focusLeftMargin && mouse.X < m.focusLeftMargin+m.focusBackWidth:
			return m.updateModal(tea.KeyPressMsg{Code: tea.KeyEsc})
		case mouse.X >= m.focusHelpX && mouse.X < m.focusHelpX+m.focusHelpWidth:
			m.pushModal(detailHelpModal())
		}
		return nil
	}

	// Inline title rename: any click while renaming commits first; a click on
	// the title text opens the editor.
	if press && m.titleEditor != nil {
		m.commitTitleEdit()
		return nil
	}
	if press && m.focusTitleWidth > 0 && mouse.Y == m.focusTitleRow &&
		mouse.X >= m.focusTitleX && mouse.X < m.focusTitleX+m.focusTitleWidth {
		m.beginTitleEdit()
		return nil
	}

	// A click on an integration code (Jira/PR) opens its URL; a click on the
	// "+ add Claude agent" affordance opens the picker.
	if press {
		for _, sp := range m.focusCodeSpans {
			if mouse.Y == m.focusContentTop+sp.line && mouse.X >= sp.x0 && mouse.X < sp.x1 {
				// Clicking a sigil focuses it (so r/c/Ctrl+X act on it), then
				// performs the click's own action below.
				if idx := m.focusLinkAtLine(sp.line); idx != noSelection {
					m.focusLinkIdx = idx
				}
				if sp.url == addAgentSentinel {
					return m.openAgentPicker()
				}
				if strings.HasPrefix(sp.url, agentFocusPrefix) {
					return m.openAgent(strings.TrimPrefix(sp.url, agentFocusPrefix))
				}
				if sp.url == forgeSentinel {
					if q := m.findQuest(mod.QuestID); q != nil {
						return m.forgePlans(q)
					}
					return nil
				}
				if sp.url == findSentinel {
					if q := m.findQuest(mod.QuestID); q != nil {
						return m.findTracksInTrails(q.ID)
					}
					return nil
				}
				if sp.url == restoreSentinel {
					if q := m.findQuest(mod.QuestID); q != nil {
						return m.restoreDismissedTracks(q.ID)
					}
					return nil
				}
				if key, ok := strings.CutPrefix(sp.url, copySectionSentinel); ok {
					if q := m.findQuest(mod.QuestID); q != nil {
						return m.copyToClipboard(m.copySection(q, key), "section copied")
					}
					return nil
				}
				if event, ok := strings.CutPrefix(sp.url, copyTrackSentinel); ok {
					return m.copyTrack(mod.QuestID, event)
				}
				// A real link: single click copies, a fast second click opens.
				return m.clickLink(sp.url)
			}
		}
	}

	// Campaign detail's "Quests" list: route a click on a quest row through the
	// SAME shared row-click handler the outline uses — so select, double-click
	// to open, and checkbox-to-toggle-done all work identically here. A click
	// above the list drops back to editing the campaign description.
	if mod.Kind == ModalCampaignDetail {
		qrows := campaignQuestRows(m.store, mod.CampaignID)
		qi := (mouse.Y - m.focusContentTop) - m.focusQuestListStart
		if qi >= 0 && qi < len(qrows) {
			if !press {
				return nil
			}
			mod.InQuestList = true
			return m.clickRowAt(qrows, qi, mouse, m.focusLeftMargin)
		}
		mod.InQuestList = false // clicked the description area
	}
	// The body column starts at focusBodyX (== the content margin for
	// single-column pages). A click to its left is in the details column, not
	// the body — the details spans above already had their chance.
	if mouse.X < m.focusBodyX {
		return nil
	}
	body := m.currentBody()
	if body == nil {
		return nil
	}
	// Soft-wrapped body lines span several screen rows — the row map built
	// during rendering resolves a row back to its body line and the raw
	// rune offset that row starts at.
	bodyRow := mouse.Y - m.focusBodyBaseRow
	if bodyRow < 0 || bodyRow >= len(m.focusRowLine) {
		return nil
	}
	bodyIdx := m.focusRowLine[bodyRow]
	if bodyIdx < 0 || bodyIdx >= len(*body) {
		return nil
	}

	// Body text starts at column 4 + 2·indent (cursor mark + indent + lead)
	// within the body column (right column in the quest detail).
	textCol := m.focusBodyX + 4 + 2*(*body)[bodyIdx].Indent

	if press {
		raw := []rune((*body)[bodyIdx].Text)
		pos := clampInt(m.focusRowOffset[bodyRow]+mouse.X-textCol, 0, len(raw))
		// A click on an objective's checkbox toggles it done — same as the Wilds.
		// The checkbox (lead glyph) occupies the two columns just before the text.
		if kind, _ := model.ClassifyBodyLine((*body)[bodyIdx].Text); kind == model.BodyObjective {
			checkboxStart := m.focusBodyX + bodyObjCol + 2*(*body)[bodyIdx].Indent
			if mouse.X >= checkboxStart && mouse.X < textCol {
				m.clearFocusLink()
				m.commitBodyLine()
				cmd := m.toggleBodyObjective(bodyIdx, checkboxStart, mouse.Y)
				mod.BodyCursor = bodyIdx
				mod.BodyEditor = m.newBodyEditor((*body)[bodyIdx].Text)
				return cmd
			}
		}
		// A click landing on a shortened inline link copies/opens it (the box
		// layout drops body-link spans, so resolve the link from the text here).
		if url := m.bodyLinkAt(bodyIdx, pos); url != "" {
			return m.clickLink(url)
		}
		m.clearFocusLink() // clicking into the body takes the caret out of the links
		m.commitBodyLine()
		if bodyIdx != mod.BodyCursor {
			mod.BodyCursor = bodyIdx
			mod.BodyEditor = bodyLineEditor(string(raw))
		}
		mod.BodyEditor.SetCursor(pos)
		m.selAnchor = pos
		m.selAnchorLine = bodyIdx
		return nil
	}

	// drag — extend the selection, following the mouse across lines. Only while
	// the left button is actually held: v2 motion events don't carry button
	// state, so without this guard mere mouse movement after a click (the anchor
	// persists past release) kept extending the selection.
	if !m.leftDown || m.selAnchor == noSelection {
		return nil
	}
	if bodyIdx != mod.BodyCursor {
		m.commitBodyLine()
		mod.BodyCursor = bodyIdx
		mod.BodyEditor = bodyLineEditor((*body)[bodyIdx].Text) // not newBodyEditor — the anchor must survive
	}
	runes := []rune(mod.BodyEditor.Value())
	mod.BodyEditor.SetCursor(clampInt(m.focusRowOffset[bodyRow]+mouse.X-textCol, 0, len(runes)))
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
	m.focusTitleWidth = 0 // set by the quest/campaign cases; 0 = title not clickable

	var b strings.Builder
	ln := 0 // lines emitted so far, so we can record where the caret lands
	emit := func(s string) {
		b.WriteString(s)
		b.WriteString("\n")
		ln++
	}

	switch mod.Kind {
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

		mark := "  "
		if m.titleEditor != nil {
			mark = ui.StyleCursor.Render(ui.GlyphCursor)
		}
		m.focusTitleX = m.focusLeftMargin + lipgloss.Width(mark)
		m.focusTitleWidth = lipgloss.Width(ui.StyleTitle.Render(name))
		// Constant width whether renaming or not, so the progress never shifts.
		emit(mark + m.constantWidthTitle(name, m.titleEditor, ui.StyleTitle, ui.StyleTitle) + progress)
		emit("")
		m.focusBodyLineStart = ln
		for i, l := range p.Body {
			editing := m.bodyCaretActive() && i == mod.BodyCursor
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
		m.focusQuestListStart = ln // content line of the first quest row (click map)
		for _, row := range campaignQuestRows(m.store, p.ID) {
			isCursor := mod.InQuestList && m.cursor.matches(row)
			if isCursor {
				m.focusCaretLine = ln
			}
			emit(m.renderFocusListRow(row, isCursor))
		}
		return strings.TrimRight(b.String(), "\n")

	case ModalSectionDetail:
		emit(ui.StyleTitle.Render(m.sectionTitle(mod.Section)))
		emit("")
		rows := m.sectionRows(mod.Section)
		if len(rows) == 0 {
			emit(ui.StyleMuted.Render("  (empty)"))
			return strings.TrimRight(b.String(), "\n")
		}
		for _, row := range rows {
			isCursor := m.cursor.matches(row)
			if isCursor {
				m.focusCaretLine = ln
			}
			emit(m.renderFocusListRow(row, isCursor))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	return ""
}

// renderFocusListRow renders one row of a focus page's list (campaign quest
// list, section page) exactly like the outline and Tavern do — shared title
// content, inline emblems, and the same delete-confirm inline prompt. One
// definition so these lists never drift from the others again.
func (m *Model) renderFocusListRow(row ui.Row, isCursor bool) string {
	warning := m.warningText != "" && m.warningTarget.matches(row)
	titleView := ""
	if warning {
		titleView = ui.StyleMuted.Render(m.warningText)
	} else {
		titleView = m.rowTitleView(row, isCursor)
	}
	hint := ""
	if isCursor && m.confirmDeleteID != "" && rowMatchesConfirmDelete(row, m.confirmDeleteID) {
		hint = "  " + ui.StyleImportant.Render(m.confirmDeleteHint(row))
	}
	line, _ := ui.RenderRow(row, m.store, titleView, isCursor, m.isNewQuest(row), 80, hint)
	if warning {
		return line
	}
	return m.withConnectionIcons(line, row)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
