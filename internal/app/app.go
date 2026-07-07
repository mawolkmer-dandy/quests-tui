package app

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/quickadd"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// viewVPad is the blank-row breathing room kept at the top and bottom of the
// main outline and the focus views (clamped down on very short terminals).
const viewVPad = 8

// mouseLeakChars are the only characters an SGR mouse report ("ESC[<b;x;yM",
// or fragments of one) is built from. Some terminals mis-deliver these as
// text — especially bursts of "[" when the wheel over-scrolls a boundary.
const mouseLeakChars = "\x1b[<;0123456789Mm"

// isMouseLeak flags a multi-rune key event whose runes are ALL from the
// mouse-report alphabet and include a "[" or "<" introducer — that can only
// be a leaked (partial) mouse sequence, never real typing. A lone "[" (or
// digits like "12", "5M") is left alone so ordinary input still works.
func isMouseLeak(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || len(msg.Runes) < 2 {
		return false
	}
	marker := false
	for _, r := range msg.Runes {
		if !strings.ContainsRune(mouseLeakChars, r) {
			return false
		}
		if r == '[' || r == '<' {
			marker = true
		}
	}
	return marker
}

// mouseAlphabetKey reports whether every rune of a key event is drawn from
// the mouse-report alphabet — used only inside the brief post-wheel window
// (see lastWheelAt), where such a keystroke can only be a leaked fragment,
// never intentional typing.
func mouseAlphabetKey(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return false
	}
	for _, r := range msg.Runes {
		if !strings.ContainsRune(mouseLeakChars, r) {
			return false
		}
	}
	return true
}

// cursorTarget identifies the row the cursor is on by identity (project ID /
// quest ID / section name) rather than raw index, so it survives the row
// list being rebuilt fresh every frame — a mutation (toggling done, e.g.)
// can move a quest to a different position in the list without losing the
// cursor.
type cursorTarget struct {
	kind      ui.RowKind
	projectID string
	questID   string
	section   string
	label     string
}

func targetFromRow(row ui.Row) cursorTarget {
	return cursorTarget{kind: row.Kind, projectID: row.ProjectID, questID: row.QuestID, section: row.Section, label: row.Label}
}

func (t cursorTarget) matches(row ui.Row) bool {
	if t.kind != row.Kind {
		return false
	}
	switch t.kind {
	case ui.RowProject:
		return t.projectID == row.ProjectID
	case ui.RowQuest:
		return t.questID == row.QuestID
	case ui.RowSection:
		return t.section == row.Section
	case ui.RowNewProject:
		return true
	case ui.RowNewQuest:
		return t.projectID == row.ProjectID
	case ui.RowLabel:
		return t.label == row.Label
	}
	return false
}

func findRowIndex(rows []ui.Row, target cursorTarget) int {
	for i, r := range rows {
		if target.matches(r) {
			return i
		}
	}
	return -1
}

type Model struct {
	store   *store.Store
	path    string
	darkBg  bool
	watcher *fsnotify.Watcher // watches the quick-add spool for live ingestion (see quickadd_watch.go)

	// Undo stack of prior store states (JSON snapshots). recordUndo pushes the
	// pre-change state on each save; undo (Ctrl+Z) pops and restores. Bounded
	// to undoLimit. applyingUndo suppresses recording while restoring.
	undoStack    [][]byte
	lastSnapshot []byte
	applyingUndo bool

	// afield is the "out on the road" view: a flat, filtered list of quests
	// (see afieldRows) instead of the full Tavern outline. quickFilter is the
	// radio chip narrowing it (All / High priority / Taken).
	afield      bool
	quickFilter quickFilter
	animate     bool // whether the intro/transition animation plays (config: intro)

	// chipLineRow is the screen row of the reserved filter line; chipSpans are
	// the Afield quick-chip click extents on it (see handleMouse).
	chipLineRow int
	chipSpans   []chipSpan

	// Inline search/filter bar (Ctrl+F) — see search.go.
	searchOpen       bool
	searchInput      textinput.Model
	searchFocus      int
	fPriority        facetPriority
	fStatus          facetStatus
	fType            facetType
	savedQuickFilter quickFilter

	width, height int
	scrollOffset  int
	subtitle      string

	// startup logo animation (see anim.go, ui.RenderLogoIntro) — plays in
	// place inside the already fully-laid-out outline; introDone switches
	// the logo over to its plain, static rendering once it finishes (or is
	// skipped by a keypress).
	introFrame int
	introDone  bool

	// set each View() call, used by handleMouse to map screen coordinates
	// back to a row index / in-row column.
	rowsScreenTop int
	leftMargin    int

	collapsedProjects map[string]bool
	collapsedSections map[string]bool

	cursor cursorTarget
	editor *textinput.Model

	// selAnchor is the other end of a text selection in whichever editor is
	// currently focused (m.editor, a modal's BodyEditor, or its
	// SearchInput) — see selection.go. noSelection means there isn't one.
	// selAnchorLine is the body-line index the anchor sits on inside a
	// focus view — when it differs from the modal's BodyCursor, the
	// selection spans lines (copy-only; see multilineSelActive).
	selAnchor     int
	selAnchorLine int

	// focusBodyBaseRow/focusLeftMargin are set each renderFocusView call, so
	// handleFocusMouse can map a click/drag back to a rune position within
	// the body lines. focusHeaderRow and the back/help extents make the
	// header's "← back (esc)" / "F1 help" clickable.
	focusBodyBaseRow int
	focusLeftMargin  int
	focusTextWidth   int
	focusHeaderRow   int
	// focusCaretLine is the 0-based line (within renderFocusContent's output)
	// the caret sits on, so renderFocusView can scroll to keep it in view.
	// focusScroll is the current vertical scroll offset of the focus view
	// (reset to 0 whenever a focus view is opened/left).
	focusCaretLine int
	focusScroll    int
	focusBackWidth int
	focusHelpX     int
	focusHelpWidth int

	// focusRowLine/focusRowOffset map each rendered body row (soft-wrapped
	// lines span several) back to its body line index and the raw rune
	// offset the row starts at — rebuilt every renderFocusContent, consumed
	// by handleFocusMouse.
	focusRowLine   []int
	focusRowOffset []int

	// hintSpans maps a visible row index to the clickable extents of its
	// rendered action hints ("→ open (tab)" etc.), rebuilt each View — a
	// click inside a span triggers that hint's action (see handleMouse).
	hintSpans map[int][]hintSpan

	// non-empty while an inline delete confirmation is armed for the row
	// under the cursor — a quest or a campaign (see handleBackspace / handleKey).
	confirmDeleteID string

	// modal is the topmost open modal; modalStack holds whatever's beneath
	// it, so drilling from a campaign's detail page into one of its quests
	// (Tab) and then closing (Esc) returns to the campaign, not the outline.
	modal      *Modal
	modalStack []*Modal

	// hover is whatever row the mouse is currently resting over, or the
	// cursor's own row (nil if neither applies) — used to show an action
	// hint ("→ open (tab)", "↓ collapse (enter)") next to it. See
	// updateHover, actionHint, and View()'s row loop. hideHoverTips
	// (Ctrl+K) suppresses those hints without affecting anything else (the
	// delete y/n prompt is driven by confirmDeleteID, a separate mechanism,
	// and always shows).
	hover         *cursorTarget
	hideHoverTips bool

	// warningText, if non-empty, replaces warningTarget's title for a couple
	// of seconds — used for "vault is read-only" when an action is blocked
	// (see showWarning in anim.go).
	warningTarget cursorTarget
	warningText   string
	warningGen    int

	// clipboardToastActive briefly swaps the footer (or the focused view's
	// header) for a "copied to clipboard" indicator right after a selection
	// is copied — see showClipboardToast in anim.go.
	clipboardToastActive bool
	clipboardToastGen    int

	// lastWheelAt is when the last scroll-wheel event arrived. Bubble Tea's
	// input parser fragments SGR mouse sequences under fast scrolling and
	// leaks the pieces as key runes (charmbracelet/bubbletea#1627); we drop
	// mouse-alphabet key events for a short window after any wheel event so
	// those stragglers can't land in the text.
	lastWheelAt time.Time

	debug     bool
	lastMsgAt time.Time
}

// Options are the config-driven behavior knobs New consumes.
type Options struct {
	QuestboardCollapsed bool
	VaultCollapsed      bool
	ShowHints           bool
	Intro               bool
	Greeting            string // empty picks a random tavern greeting
}

func New(s *store.Store, path string, darkBg bool, opts Options) *Model {
	subtitle := opts.Greeting
	if subtitle == "" {
		subtitle = ui.RandomGreeting()
	}
	m := &Model{
		store:             s,
		path:              path,
		darkBg:            darkBg,
		subtitle:          subtitle,
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{"inbox": opts.QuestboardCollapsed, "someday": opts.VaultCollapsed},
		selAnchor:         noSelection,
		selAnchorLine:     noSelection,
		hideHoverTips:     !opts.ShowHints,
		introDone:         !opts.Intro,
		animate:           opts.Intro,
		lastSnapshot:      s.Snapshot(),
	}
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	}
	return m
}

// SetDebug turns on per-message timing logs (QUESTS_DEBUG in main.go).
func (m *Model) SetDebug(on bool) {
	m.debug = on
}

func (m *Model) Init() tea.Cmd {
	// The splash ticker starts from the first WindowSizeMsg instead (see
	// Update) so it doesn't burn frames before there's a size to render into.
	// Start watching the quick-add spool so captures made elsewhere (CLI,
	// Raycast) show up live without a relaunch.
	return m.watchQuickAdd()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.debug {
		now := time.Now()
		gap := now.Sub(m.lastMsgAt)
		m.lastMsgAt = now
		log.Printf("update: %T %+v (gap since last msg: %s)", msg, msg, gap)
	}

	switch msg := msg.(type) {
	case quickAddMsg:
		// A capture landed in the spool while we're running — ingest it and
		// keep listening. Cursor is tracked by identity, so appending quests
		// doesn't disturb it; the new row appears on the next render.
		if m.watcher != nil {
			if n := quickadd.Drain(filepath.Dir(m.path), m.store); n > 0 {
				m.save()
			}
			return m, waitForQuickAdd(m.watcher)
		}
		return m, nil

	case tea.WindowSizeMsg:
		// The splash's frame ticker only starts once we know the terminal
		// size — starting it from Init() ticks in the dark until the first
		// WindowSizeMsg arrives (which can lag on some terminals), burning
		// most of the animation before there's anything to render it into.
		firstSize := m.width == 0
		m.width, m.height = msg.Width, msg.Height
		if firstSize && !m.introDone {
			return m, introTick()
		}
		return m, nil

	case introTickMsg:
		return m, m.advanceIntro()

	case warningExpireMsg:
		m.clearWarningIfCurrent(msg.gen)
		return m, nil

	case clipboardToastExpireMsg:
		m.clearClipboardToastIfCurrent(msg.gen)
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		// Some terminals/multiplexers occasionally feed unparsed mouse-report
		// escapes (e.g. "[<65;80;30M" from scroll wheels) into the key stream;
		// drop them so they can't get typed into a title or body line. Right
		// after a wheel event, also drop any lone mouse-alphabet key (a "["
		// fragment fast scrolling split off), which the structural check
		// above can't tell from a real keystroke on its own.
		if isMouseLeak(msg) {
			return m, nil
		}
		if time.Since(m.lastWheelAt) < 300*time.Millisecond && mouseAlphabetKey(msg) {
			return m, nil
		}
		// The intro/transition animation is non-blocking: keys pass straight
		// through and act normally while it plays (it finishes on its own).
		if m.modal != nil {
			if isFocusModal(m.modal.Kind) && key.Matches(msg, Keys.Help) {
				m.pushModal(detailHelpModal())
				return m, nil
			}
			return m, m.updateModal(msg)
		}
		return m, m.handleKey(msg)

	case tea.MouseMsg:
		if !m.introDone {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			m.lastWheelAt = time.Now()
		}
		if m.modal != nil {
			if isFocusModal(m.modal.Kind) {
				return m, m.handleFocusMouse(msg)
			}
			return m, nil
		}
		return m, m.handleMouse(msg)
	}

	return m, nil
}

func (m *Model) findProject(id string) *model.Project {
	for i := range m.store.Projects {
		if m.store.Projects[i].ID == id {
			return &m.store.Projects[i]
		}
	}
	return nil
}

func (m *Model) findQuest(id string) *model.Quest {
	for i := range m.store.Quests {
		if m.store.Quests[i].ID == id {
			return &m.store.Quests[i]
		}
	}
	return nil
}

func (m *Model) projectQuestCount(id string) int {
	n := 0
	for _, q := range m.store.Quests {
		if q.ProjectID == id {
			n++
		}
	}
	return n
}

const undoLimit = 100

func (m *Model) save() {
	if !m.applyingUndo {
		m.recordUndo()
	}
	_ = store.Save(m.path, m.store)
}

// recordUndo pushes the last snapshot onto the undo stack when the store has
// actually changed since it was taken, then remembers the new state. Called
// from save() (after the mutation), so the pushed snapshot is the pre-change
// state that Ctrl+Z restores.
func (m *Model) recordUndo() {
	cur := m.store.Snapshot()
	if bytes.Equal(cur, m.lastSnapshot) {
		return
	}
	if m.lastSnapshot != nil {
		m.undoStack = append(m.undoStack, m.lastSnapshot)
		if len(m.undoStack) > undoLimit {
			m.undoStack = m.undoStack[len(m.undoStack)-undoLimit:]
		}
	}
	m.lastSnapshot = cur
}

// undo restores the most recent pre-change store snapshot. Cursor is kept if
// its row still exists, else it lands on the nearest surviving row.
func (m *Model) undo() {
	if len(m.undoStack) == 0 {
		return
	}
	prev := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]

	restored, err := store.RestoreSnapshot(prev)
	if err != nil {
		return
	}
	*m.store = *restored
	m.lastSnapshot = prev

	m.applyingUndo = true
	m.save()
	m.applyingUndo = false

	m.editor = nil
	m.clearSelection()
	rows := m.visibleRows()
	if row, ok := nearestSelectableRow(rows, findRowIndex(rows, m.cursor)); ok {
		m.setCursor(row)
	}
}

// pushModal opens next, keeping whatever's currently open (if anything) on
// the stack beneath it.
func (m *Model) pushModal(next *Modal) {
	if m.modal != nil {
		m.modalStack = append(m.modalStack, m.modal)
	}
	m.modal = next
	m.hover = nil // mouse is disabled while any modal is open; don't leave a stale hint behind
	m.clearSelection()
	m.focusScroll = 0 // a freshly opened focus view starts at the top
}

// closeModal closes the current modal, returning to whatever was beneath it
// on the stack (or to the outline if nothing was).
func (m *Model) closeModal() {
	// A selection made in the field being left behind (a body line, the
	// search box) must not leak into whatever's focused next — its anchor
	// is just a rune index, meaningless (and potentially out of bounds) for
	// a different, unrelated textinput.
	m.clearSelection()
	m.focusScroll = 0 // re-entering a view below re-scrolls to its caret
	if n := len(m.modalStack); n > 0 {
		m.modal = m.modalStack[n-1]
		m.modalStack = m.modalStack[:n-1]
		return
	}
	m.modal = nil
}

// visibleRows is the row list every navigation/mutation/render path should
// use — it applies the live search filter on top of ui.BuildRows so a
// filtered view can't be navigated "past" into hidden rows.
// chipSpan is a quick-chip's clickable extent in absolute screen columns.
type chipSpan struct {
	x0, x1 int
	filter quickFilter
}

// renderFilterLine renders the reserved line above the rows: the Afield quick
// chips, or blank (the Tavern, and the search bar, come from elsewhere). It
// records chipSpans for click hit-testing.
func (m *Model) renderFilterLine(width int, margin string) string {
	m.chipSpans = nil
	if m.searchOpen {
		return margin + m.renderSearchBar()
	}
	if !m.afield {
		return ""
	}
	filters := []quickFilter{filterAll, filterHigh, filterTaken}
	var b strings.Builder
	x := m.leftMargin
	for i, f := range filters {
		if i > 0 {
			b.WriteString("  ")
			x += 2
		}
		label := " " + f.label() + " "
		w := lipgloss.Width(label)
		m.chipSpans = append(m.chipSpans, chipSpan{x0: x, x1: x + w, filter: f})
		x += w
		if f == m.quickFilter {
			b.WriteString(ui.StyleSelectedRow.Render(label))
		} else {
			b.WriteString(ui.StyleMuted.Render(label))
		}
	}
	return margin + b.String()
}

// cycleQuickFilter steps the Afield chip left/right and re-homes the cursor.
func (m *Model) cycleQuickFilter(delta int) {
	const n = 3
	m.quickFilter = quickFilter((int(m.quickFilter) + delta + n) % n)
	m.scrollOffset = 0
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	} else {
		m.cursor = cursorTarget{}
	}
}

// setQuickFilter selects a chip directly (from a mouse click).
func (m *Model) setQuickFilter(f quickFilter) {
	if m.quickFilter == f {
		return
	}
	m.quickFilter = f
	m.scrollOffset = 0
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	} else {
		m.cursor = cursorTarget{}
	}
}

// quickFilter is the Afield radio chip narrowing the flat list.
type quickFilter int

const (
	filterAll quickFilter = iota
	filterHigh
	filterTaken
)

func (f quickFilter) label() string {
	switch f {
	case filterHigh:
		return "High priority"
	case filterTaken:
		return "Taken"
	default:
		return "All"
	}
}

// quickFilterMatch reports whether q passes the active Afield chip.
func (m *Model) quickFilterMatch(q *model.Quest) bool {
	switch m.quickFilter {
	case filterHigh:
		return q.Priority == model.PriorityHigh
	case filterTaken:
		return q.Status == model.StatusActive
	default:
		return true
	}
}

// afieldRows is the flat quest list shown out on the road: every non-done
// quest under a non-archived campaign that passes the quick filter, in the
// same tiered order the outline uses, tagged with its campaign name. No
// Questboard/Vault, no headers, no "+ New" affordances — those are Tavern
// activities.
func (m *Model) afieldRows() []ui.Row {
	var rows []ui.Row
	for i := range m.store.Projects {
		p := &m.store.Projects[i]
		if p.Archived {
			continue
		}
		for _, q := range ui.QuestsForCampaign(m.store, p.ID) {
			if q.Status == model.StatusDone || !m.quickFilterMatch(&q) {
				continue
			}
			if m.searchOpen && !m.searchMatch(&q) {
				continue
			}
			rows = append(rows, ui.Row{Kind: ui.RowQuest, ProjectID: p.ID, QuestID: q.ID, ShowProjectTag: true})
		}
	}
	return rows
}

// setAfield switches between the Tavern and Afield views, replaying the
// intro transition and rerolling the flavor subtitle for the destination.
// Setting out defaults the quick chip to Taken (your active quests).
func (m *Model) setAfield(on bool) tea.Cmd {
	if m.afield == on {
		return nil
	}
	m.commitEdit()
	m.afield = on
	m.editor = nil
	m.scrollOffset = 0
	if on {
		m.quickFilter = filterTaken
		m.subtitle = ui.RandomAfieldGreeting()
	} else {
		m.subtitle = ui.RandomGreeting()
	}
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	} else {
		m.cursor = cursorTarget{}
	}
	if !m.animate {
		return nil
	}
	m.introFrame = 0
	m.introDone = false
	return introTick()
}

func (m *Model) visibleRows() []ui.Row {
	if m.afield {
		return m.afieldRows()
	}
	if !m.searchOpen {
		return ui.BuildRows(m.store, m.collapsedProjects, m.collapsedSections)
	}
	// While searching, ignore collapse state so a match in any campaign or
	// section still surfaces (empty groups are dropped below).
	rows := ui.BuildRows(m.store, map[string]bool{}, map[string]bool{})

	keep := make([]bool, len(rows))
	for i, r := range rows {
		if r.Kind == ui.RowQuest {
			if q := m.findQuest(r.QuestID); q != nil && m.searchMatch(q) {
				keep[i] = true
			}
		}
	}

	var out []ui.Row
	i := 0
	for i < len(rows) {
		r := rows[i]
		if r.Kind == ui.RowProject || r.Kind == ui.RowSection {
			j := i + 1
			groupHasMatch := false
			for j < len(rows) && rows[j].Kind == ui.RowQuest {
				if keep[j] {
					groupHasMatch = true
				}
				j++
			}
			if groupHasMatch {
				out = append(out, r)
				for k := i + 1; k < j; k++ {
					if keep[k] {
						out = append(out, rows[k])
					}
				}
			}
			i = j
			continue
		}
		i++ // RowNewProject: not relevant while searching
	}
	return out
}

// setCursor moves the cursor to row and (re)seeds the live editor for it —
// the row under the cursor is always editable text unless it's a section
// header or the "+ New Project" affordance.
func (m *Model) setCursor(row ui.Row) {
	m.cursor = targetFromRow(row)
	m.clearSelection()

	switch row.Kind {
	case ui.RowProject:
		p := m.findProject(row.ProjectID)
		if p == nil {
			m.editor = nil
			return
		}
		ti := textinput.New()
		ti.Prompt = ""
		ti.SetValue(p.Name)
		ti.CursorEnd()
		_ = ti.Focus()
		m.editor = &ti
	case ui.RowQuest:
		q := m.findQuest(row.QuestID)
		if q == nil {
			m.editor = nil
			return
		}
		ti := textinput.New()
		ti.Prompt = ""
		ti.SetValue(q.Title)
		ti.CursorEnd()
		_ = ti.Focus()
		m.editor = &ti
	default:
		m.editor = nil
	}
}

// commitEdit writes the live editor's value back to whatever the cursor
// currently targets. Call before moving the cursor anywhere else.
func (m *Model) commitEdit() {
	if m.editor == nil {
		return
	}
	value := strings.TrimSpace(m.editor.Value())
	switch m.cursor.kind {
	case ui.RowProject:
		if p := m.findProject(m.cursor.projectID); p != nil {
			p.Name = value
		}
	case ui.RowQuest:
		if q := m.findQuest(m.cursor.questID); q != nil {
			q.Title = value
			q.UpdatedAt = time.Now()
		}
	}
	m.save()
}

// removeCurrentRow deletes the cursor's row via fn, then relocates the
// cursor to the previous selectable row within currentRowScope — never
// arbitrarily to the top, and never spilling out into the rest of the
// outline when the cursor is inside a focused campaign's quest list.
func (m *Model) removeCurrentRow(fn func()) {
	rows := m.currentRowScope()
	idx := findRowIndex(rows, m.cursor)
	fn()

	newRows := m.currentRowScope()
	if row, ok := nearestSelectableRow(newRows, idx-1); ok {
		m.setCursor(row)
		return
	}
	m.editor = nil
}

// currentRowScope is the row list cursor navigation/removal should operate
// against — the full outline normally, or just one campaign's quest section
// (its quests plus "+ New Quest") while focused on that campaign's quest
// list, so deleting a quest there can't relocate the cursor out into the
// rest of the outline underneath.
func (m *Model) currentRowScope() []ui.Row {
	if m.modal != nil {
		switch {
		case m.modal.Kind == ModalCampaignDetail && m.modal.InQuestList:
			return campaignQuestRows(m.store, m.modal.CampaignID)
		case m.modal.Kind == ModalSectionDetail:
			return sectionRows(m.store, m.modal.Section)
		}
	}
	return m.visibleRows()
}

// toggleAllCampaigns collapses every campaign if any is currently expanded,
// or expands them all if every one is already collapsed — the reactive
// action behind Enter on the "Campaigns" label (see RenderRow's RowLabel
// case for the matching hint text).
func (m *Model) toggleAllCampaigns() {
	anyExpanded := false
	for _, p := range m.store.Projects {
		if !p.Archived && !m.collapsedProjects[p.ID] {
			anyExpanded = true
			break
		}
	}
	for _, p := range m.store.Projects {
		if !p.Archived {
			m.collapsedProjects[p.ID] = anyExpanded
		}
	}
}

// nearestSelectableRow finds the closest selectable row to idx, preferring
// to search backward first (so callers land on "the previous line") and
// falling back to searching forward if nothing selectable precedes it.
func nearestSelectableRow(rows []ui.Row, idx int) (ui.Row, bool) {
	if len(rows) == 0 {
		return ui.Row{}, false
	}
	if idx >= len(rows) {
		idx = len(rows) - 1
	}
	if idx < 0 {
		idx = 0
	}
	for i := idx; i >= 0; i-- {
		if rows[i].Selectable() {
			return rows[i], true
		}
	}
	for i := idx + 1; i < len(rows); i++ {
		if rows[i].Selectable() {
			return rows[i], true
		}
	}
	return ui.Row{}, false
}

func rowMatchesConfirmDelete(row ui.Row, id string) bool {
	switch row.Kind {
	case ui.RowQuest:
		return row.QuestID == id
	case ui.RowProject:
		return row.ProjectID == id
	}
	return false
}

// confirmDeleteHint is the minimal inline y/n prompt for whatever's armed.
func (m *Model) confirmDeleteHint(row ui.Row) string {
	switch row.Kind {
	case ui.RowQuest:
		return "delete this quest? y/n"
	case ui.RowProject:
		if n := m.projectQuestCount(row.ProjectID); n > 0 {
			return fmt.Sprintf("delete this campaign and its %d quest(s)? y/n", n)
		}
		return "delete this campaign? y/n"
	}
	return ""
}

func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}

	if m.modal != nil {
		if isFocusModal(m.modal.Kind) {
			return m.renderFocusView()
		}
		return m.renderModal()
	}

	contentWidth := m.width - 4
	if contentWidth > 80 {
		contentWidth = 80
	}
	if contentWidth < 20 {
		contentWidth = 20
	}
	m.leftMargin = (m.width - contentWidth) / 2
	if m.leftMargin < 0 {
		m.leftMargin = 0
	}
	margin := strings.Repeat(" ", m.leftMargin)

	rawFooter := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Right).Render(m.renderFooter())
	footer := indentLines(rawFooter, margin)
	availableHeight := m.height - lipgloss.Height(footer)
	if availableHeight < 1 {
		availableHeight = 1
	}

	logoLines := ui.RenderLogo(contentWidth, m.subtitle)
	if !m.introDone {
		logoLines = ui.RenderLogoIntro(contentWidth, m.subtitle, m.introFrame)
	}
	logoHeight := len(logoLines) + 2 // +1 blank line after the logo, +1 reserved filter/chip line

	// Keep viewVPad blank rows top and bottom, but never let the padding eat
	// more than half the screen (so short terminals stay usable). The logo +
	// rows block is centered within the region between them.
	vpad := viewVPad
	if maxPad := availableHeight / 4; vpad > maxPad {
		vpad = maxPad
	}
	if vpad < 0 {
		vpad = 0
	}
	innerHeight := availableHeight - 2*vpad
	if innerHeight < 1 {
		innerHeight = 1
	}
	viewHeight := innerHeight - logoHeight
	if viewHeight < 1 {
		viewHeight = 1
	}

	rows := m.visibleRows()
	idx := findRowIndex(rows, m.cursor)
	if idx < 0 && len(rows) > 0 {
		idx = 0
		m.setCursor(rows[0])
	}

	if idx >= 0 {
		if idx < m.scrollOffset {
			m.scrollOffset = idx
		}
		if idx >= m.scrollOffset+viewHeight {
			m.scrollOffset = idx - viewHeight + 1
		}
	}
	maxScroll := len(rows) - viewHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	end := m.scrollOffset + viewHeight
	if end > len(rows) {
		end = len(rows)
	}
	shown := end - m.scrollOffset
	if shown < 0 {
		shown = 0
	}

	blockHeight := logoHeight + shown
	topPad := vpad + (innerHeight-blockHeight)/2
	if topPad < 0 {
		topPad = 0
	}
	bottomPad := availableHeight - blockHeight - topPad
	if bottomPad < 0 {
		bottomPad = 0
	}
	m.rowsScreenTop = topPad + logoHeight
	// The reserved filter/chip line sits just above the rows (after the logo
	// and its blank line) — its screen row is used for chip click hit-testing.
	m.chipLineRow = topPad + len(logoLines) + 1

	m.hintSpans = map[int][]hintSpan{}
	hoverIdx := -1
	if m.hover != nil {
		hoverIdx = findRowIndex(rows, *m.hover)
	}
	vaultIdx := -1
	for i, r := range rows {
		if r.Kind == ui.RowSection && r.Section == "someday" {
			vaultIdx = i
			break
		}
	}

	clip := lipgloss.NewStyle().MaxWidth(m.width)
	var b strings.Builder
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}
	for _, line := range logoLines {
		b.WriteString(margin + line + "\n")
	}
	// The blank row between the logo and the rows doubles as a "more above"
	// hint when the list is scrolled down past its start.
	if m.scrollOffset > 0 {
		b.WriteString(foldHint(margin, contentWidth) + "\n")
	} else {
		b.WriteString("\n")
	}
	// The reserved filter line: Afield quick chips, the search bar when open,
	// or blank — always present so toggling it never reflows the list.
	b.WriteString(clip.Render(m.renderFilterLine(contentWidth, margin)) + "\n")
	for i := m.scrollOffset; i < end; i++ {
		row := rows[i]
		isCursor := i == idx
		confirming := isCursor && m.confirmDeleteID != "" && rowMatchesConfirmDelete(row, m.confirmDeleteID)
		warning := m.warningText != "" && m.warningTarget.matches(row)
		titleView := ""
		if warning {
			titleView = ui.StyleMuted.Render(m.warningText)
		} else if isCursor && m.editor != nil {
			titleView = m.renderEditableStyled(m.editor, m.cursorTitleStyle(row))
		}
		hint := ""
		var hintParts []hintPart
		if confirming {
			// Keep the name visible; the prompt takes over the right-hand
			// hint slot (where open/collapse tips would be) so you can see
			// exactly what you're about to delete.
			hint = "  " + ui.StyleImportant.Render(m.confirmDeleteHint(row))
		} else if !warning {
			if !m.hideHoverTips && (isCursor || hoverIdx == i) {
				hintParts = actionHintParts(row)
			}
			hint = renderHintParts(hintParts)
			if !m.hideHoverTips && row.Kind == ui.RowSection && row.Section == "someday" && vaultIdx >= 0 && hoverIdx >= vaultIdx {
				hint += "  " + ui.StyleMuted.Render("(read only)")
			}
		}
		// RenderRow places the hint inline right after the row's content
		// (before a campaign's right-aligned progress) and reports where —
		// that's what makes the hint parts clickable.
		rendered, hintX := ui.RenderRow(row, m.store, titleView, isCursor, contentWidth, hint)
		if len(hintParts) > 0 && hintX >= 0 {
			x := m.leftMargin + hintX + 2 // +2 for the gap renderHintParts prepends
			var spans []hintSpan
			for _, p := range hintParts {
				w := lipgloss.Width(p.label)
				spans = append(spans, hintSpan{x0: x, x1: x + w, action: p.action})
				x += w + 2 // labels are joined with two spaces
			}
			m.hintSpans[i] = spans
		}
		b.WriteString(clip.Render(margin + rendered))
		b.WriteString("\n")
	}
	for i := 0; i < bottomPad; i++ {
		// First row of the bottom padding signals "more below".
		if i == 0 && end < len(rows) {
			b.WriteString(foldHint(margin, contentWidth) + "\n")
			continue
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n") + "\n" + footer
}

// hintPart is one "<icon> <verb> (<key>)" action tip; action names the key
// it stands in for ("enter"/"tab"), so a mouse click on the rendered label
// can trigger the same thing (see hintSpan / handleMouse).
type hintPart struct {
	label  string
	action string
}

// hintSpan is a hint part's clickable extent in absolute screen columns.
type hintSpan struct {
	x0, x1 int
	action string
}

// actionHintParts lists the action tips for row — e.g. collapse + open for
// a campaign, just open for a quest.
func actionHintParts(row ui.Row) []hintPart {
	switch row.Kind {
	case ui.RowProject:
		return []hintPart{{collapseHint(row.Collapsed), "enter"}, {"→ open (tab)", "tab"}}
	case ui.RowSection:
		return []hintPart{{collapseHint(row.Collapsed), "enter"}, {"→ open (tab)", "tab"}}
	case ui.RowQuest:
		return []hintPart{{"→ open (tab)", "tab"}}
	case ui.RowLabel:
		if row.Collapsed {
			return []hintPart{{"↓ expand all (enter)", "enter"}}
		}
		return []hintPart{{"↑ collapse all (enter)", "enter"}}
	}
	return nil
}

// renderHintParts renders the joined muted hint text, prefixed with the
// two-space gap that separates it from the row's own content.
func renderHintParts(parts []hintPart) string {
	if len(parts) == 0 {
		return ""
	}
	labels := make([]string, len(parts))
	for i, p := range parts {
		labels[i] = p.label
	}
	return "  " + ui.StyleMuted.Render(strings.Join(labels, "  "))
}

// actionHint returns the rendered tip(s) for row when it's under attention
// (active — the keyboard cursor or a mouse hover, either counts, so the
// hint is discoverable with just a keyboard). Hidden entirely via Ctrl+K
// (hideHoverTips).
func (m *Model) actionHint(row ui.Row, active bool) string {
	if m.hideHoverTips || !active {
		return ""
	}
	return renderHintParts(actionHintParts(row))
}

func collapseHint(collapsed bool) string {
	if collapsed {
		return "↓ expand (enter)"
	}
	return "↑ collapse (enter)"
}

// renderFooter is deliberately just a short, right-aligned pointer to the
// help overlay — not an inline dump of every keybinding.
func (m *Model) renderFooter() string {
	if m.clipboardToastActive {
		return lipgloss.NewStyle().Padding(0, 1).Render(renderClipboardToast())
	}
	if m.afield {
		return ui.StyleFooter.Render("⚔ afield · Ctrl+G return to the tavern")
	}
	taken := m.takenCount()
	setOut := "⚔ Ctrl+G set out"
	if taken > 0 {
		setOut = fmt.Sprintf("⚔ Ctrl+G set out · %d taken up", taken)
	}
	return ui.StyleFooter.Render(setOut)
}

// takenCount is how many quests are currently taken up (active) under a
// non-archived campaign — the number you'd take Afield.
func (m *Model) takenCount() int {
	n := 0
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		if q.Status == model.StatusActive && q.ProjectID != "" && !q.Vaulted {
			if p := m.findProject(q.ProjectID); p != nil && !p.Archived {
				n++
			}
		}
	}
	return n
}

// indentLines prepends prefix to every line of s (a possibly multi-line,
// already-wrapped block), not just the first.
func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
