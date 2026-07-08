package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The inline search/filter bar (Ctrl+F) — a fuzzy name find plus priority /
// status / type facet chips, shown on the reserved line in both views. It
// replaces the old search modal. While open it filters the current view;
// closing it clears the filter (and, in Afield, restores the quick chip that
// was active before it opened).

type facetPriority int

const (
	fpAny facetPriority = iota
	fpHigh
	fpMedium
	fpLow
)

type facetStatus int

const (
	fsAny facetStatus = iota
	fsTaken
	fsOpen
	fsDone
)

type facetType int

const (
	ftAny facetType = iota
	ftMain
	ftSide
)

// searchFocus values — which element of the bar the keyboard drives.
const (
	focusText = iota
	focusPriority
	focusStatus
	focusType
	focusCount
)

func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "find a campaign or quest…"
	_ = ti.Focus()
	return ti
}

// openSearch shows the bar and focuses the text field. In Afield it parks the
// quick chip on All (so the search spans everything) and remembers the old one
// to restore on close.
func (m *Model) openSearch() {
	if m.searchOpen {
		return
	}
	m.commitEdit()
	m.searchOpen = true
	m.searchFocus = focusText
	m.searchInput = newSearchInput()
	m.fPriority, m.fStatus, m.fType = fpAny, fsAny, ftAny
	if m.afield {
		m.savedQuickFilter = m.quickFilter
		m.quickFilter = filterAll
	}
	m.rehomeCursor()
}

// closeSearch hides the bar and clears the filter — back to All in the Tavern,
// or the pre-search quick chip in Afield.
func (m *Model) closeSearch() {
	if !m.searchOpen {
		return
	}
	m.searchOpen = false
	m.searchInput.Blur()
	if m.afield {
		m.quickFilter = m.savedQuickFilter
	}
	m.rehomeCursor()
}

// rehomeCursor resets scroll and puts the cursor on the first visible row —
// used whenever the filtered row set changes out from under it.
func (m *Model) rehomeCursor() {
	m.scrollOffset = 0
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	} else {
		m.cursor = cursorTarget{}
		m.editor = nil
	}
}

// searchMatch reports whether a quest passes the open search bar (fuzzy name
// against its title or campaign, AND each set facet). Only consulted while the
// bar is open.
func (m *Model) searchMatch(q *model.Quest) bool {
	if s := strings.TrimSpace(m.searchInput.Value()); s != "" {
		hit := fuzzySubsequence(s, q.Title)
		if !hit {
			if p := m.findProject(q.ProjectID); p != nil && fuzzySubsequence(s, p.Name) {
				hit = true
			}
		}
		if !hit {
			return false
		}
	}
	switch m.fPriority {
	case fpHigh:
		if q.Priority != model.PriorityHigh {
			return false
		}
	case fpMedium:
		if q.Priority != model.PriorityMedium {
			return false
		}
	case fpLow:
		if q.Priority != model.PriorityLow {
			return false
		}
	}
	switch m.fStatus {
	case fsTaken:
		if q.Status != model.StatusActive {
			return false
		}
	case fsOpen:
		if q.Status != model.StatusOpen {
			return false
		}
	case fsDone:
		if q.Status != model.StatusDone {
			return false
		}
	}
	switch m.fType {
	case ftMain:
		if q.Type != model.QuestTypeMain {
			return false
		}
	case ftSide:
		if q.Type != model.QuestTypeSide {
			return false
		}
	}
	return true
}

func (m *Model) cycleFacet(focus, delta int) {
	switch focus {
	case focusPriority:
		m.fPriority = facetPriority((int(m.fPriority) + delta + 4) % 4)
	case focusStatus:
		m.fStatus = facetStatus((int(m.fStatus) + delta + 4) % 4)
	case focusType:
		m.fType = facetType((int(m.fType) + delta + 3) % 3)
	}
	m.rehomeCursor()
}

// handleSearchBarKey intercepts the keys the bar owns while it's open. Left/
// Right move focus across [text · prio · status · type]; Enter toggles the
// focused select (Tab is left free to open the cursor's quest); typing always
// edits the find field. Up/Down and command chords fall through so you can
// still navigate and act on the filtered results.
func (m *Model) handleSearchBarKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case msg.Type == tea.KeyEsc:
		m.closeSearch()
		return nil, true
	case key.Matches(msg, Keys.Search):
		m.closeSearch()
		return nil, true
	case msg.Type == tea.KeyLeft:
		m.searchFocus = (m.searchFocus + focusCount - 1) % focusCount
		return nil, true
	case msg.Type == tea.KeyRight:
		m.searchFocus = (m.searchFocus + 1) % focusCount
		return nil, true
	case msg.Type == tea.KeyEnter:
		if m.searchFocus == focusText {
			return m.handleReveal(), true // open the cursor's quest
		}
		m.cycleFacet(m.searchFocus, 1)
		return nil, true
	case msg.Type == tea.KeyRunes, msg.Type == tea.KeySpace,
		msg.Type == tea.KeyBackspace, msg.Type == tea.KeyDelete:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.rehomeCursor()
		return cmd, true
	}
	return nil, false
}

func (f facetPriority) label() string {
	return "prio:" + [...]string{"any", "high", "med", "low"}[f]
}
func (f facetStatus) label() string {
	return "status:" + [...]string{"any", "taken", "open", "done"}[f]
}
func (f facetType) label() string {
	return "type:" + [...]string{"any", "main", "side"}[f]
}

// renderSearchBar renders the bar contents for the reserved line (without the
// left margin, which the caller prepends).
func (m *Model) renderSearchBar() string {
	chip := func(focused bool, s string) string {
		if focused {
			return ui.StyleSelectedRow.Render(" " + s + " ")
		}
		return ui.StyleMuted.Render(" " + s + " ")
	}
	var input string
	if m.searchFocus == focusText {
		input = m.renderEditableText(&m.searchInput)
	} else {
		input = m.searchInput.Value()
	}
	if input == "" {
		input = ui.StyleMuted.Render(m.searchInput.Placeholder)
	}
	parts := []string{
		ui.StyleMuted.Render("⌕ ") + input,
		chip(m.searchFocus == focusPriority, m.fPriority.label()),
		chip(m.searchFocus == focusStatus, m.fStatus.label()),
		chip(m.searchFocus == focusType, m.fType.label()),
	}
	return strings.Join(parts, "  ")
}
