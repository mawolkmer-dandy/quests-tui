package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The Tavern is a borderless left "hall" (a directory of rooms, banners →
// campaigns, and secondary doors) beside a floating content pane. It's
// navigated master-detail: the cursor walks the hall (hallFocus), the pane
// live-previews whatever's selected, and Tab/→/Enter dives into the pane to act
// on its quests (Esc returns to the hall). This replaces the old top door-strip.

// hallSpan records a hall entry's screen row and identity for click routing.
type hallSpan struct {
	y      int
	target cursorTarget
}

// unassignedBanner is the sentinel BannerID for the "Unassigned" area that
// gathers campaigns not filed under any real banner.
const unassignedBanner = ""

// hallRows is the left directory: the rooms group on top (Questboard · Vault ·
// Runes · Lookouts, the latter two only when non-empty), then the areas —
// each Banner with its campaigns nested beneath, ungrouped campaigns gathered
// under an "Unassigned" area, and the "+ New" affordances at the base. Groups
// are separated by blank rows (no rules) for a calmer, roomier hall.
func (m *Model) hallRows() []ui.Row {
	s := m.store
	blank := ui.Row{Kind: ui.RowSpacer}
	rows := []ui.Row{{Kind: ui.RowSection, Section: "inbox"}}
	if ui.CountRunes(s) > 0 {
		rows = append(rows, ui.Row{Kind: ui.RowSection, Section: "runes"})
	}
	if ui.CountLookouts(s) > 0 {
		rows = append(rows, ui.Row{Kind: ui.RowSection, Section: "lookouts"})
	}
	rows = append(rows, ui.Row{Kind: ui.RowSection, Section: "someday"})

	// Areas: a blank line separates each area; the campaigns within stay tight
	// together beneath their header (the Things sidebar look).
	appendArea := func(bannerID, label string) {
		rows = append(rows, blank, ui.Row{Kind: ui.RowBanner, BannerID: bannerID, Label: label})
		for i := range s.Projects {
			if p := s.Projects[i]; !p.Archived && p.BannerID == bannerID {
				rows = append(rows, ui.Row{Kind: ui.RowProject, ProjectID: p.ID, Nested: true})
			}
		}
	}
	for _, b := range s.Banners {
		appendArea(b.ID, b.Name)
	}
	for i := range s.Projects {
		if p := s.Projects[i]; !p.Archived && p.BannerID == unassignedBanner {
			appendArea(unassignedBanner, "Unassigned")
			break
		}
	}
	// No "+ New" rows: Ctrl+N adds a Banner, Enter adds a campaign under the
	// selected area (see the footer shortcuts).
	return rows
}

// paneRows is the content of whatever the hall has selected — a room's list, a
// banner's area overview, or a campaign's quests. This IS visibleRows() in the
// Tavern, so the cursor and every action operate on it.
func (m *Model) paneRows() []ui.Row {
	switch m.hallCursor.kind {
	case ui.RowSection:
		return m.sectionRows(m.hallCursor.section)
	case ui.RowBanner:
		return m.bannerPaneRows(m.hallCursor.bannerID)
	case ui.RowProject:
		return m.campaignPaneRows(m.hallCursor.projectID)
	}
	return nil
}

// campaignPaneRows is a campaign's view: its editable name/progress header, then
// its quests grouped Active / Later / Vaulted — the same status split the banner
// uses one level up. (A campaign's description/notes editor is the retiring
// modal for now; the quest list is the fluid part.)
func (m *Model) campaignPaneRows(projectID string) []ui.Row {
	s := m.store
	rows := []ui.Row{{Kind: ui.RowProject, ProjectID: projectID, Bare: true, Header: true}, {Kind: ui.RowSpacer}}

	var activeQ, laterQ, vaultedQ []string
	for _, q := range ui.QuestsForCampaign(s, projectID) { // sorted, excludes vaulted
		if q.Status == model.StatusActive {
			activeQ = append(activeQ, q.ID)
		} else {
			laterQ = append(laterQ, q.ID) // open backlog + done-not-yet-vaulted
		}
	}
	for i := range s.Quests {
		if q := &s.Quests[i]; q.ProjectID == projectID && q.Vaulted {
			vaultedQ = append(vaultedQ, q.ID)
		}
	}

	quest := func(id string, dim bool) ui.Row {
		return ui.Row{Kind: ui.RowQuest, ProjectID: projectID, QuestID: id, Bare: true, Dim: dim}
	}

	switch {
	case len(activeQ) == 0 && len(laterQ) == 0:
		// Empty campaign → offer the first quest.
		rows = append(rows, ui.Row{Kind: ui.RowNewQuest, ProjectID: projectID, Label: "+ add quest"})
	case len(activeQ) == 0:
		// Everything inactive → one flat list, no "Later" split, not dimmed.
		for _, id := range laterQ {
			rows = append(rows, quest(id, false))
		}
	default:
		// A mix → active quests, then a dimmed "Later" backlog.
		for _, id := range activeQ {
			rows = append(rows, quest(id, false))
		}
		if len(laterQ) > 0 {
			rows = append(rows, ui.Row{Kind: ui.RowSpacer}, ui.Row{Kind: ui.RowDayHeader, Label: "Later"}, ui.Row{Kind: ui.RowSpacer})
			for _, id := range laterQ {
				rows = append(rows, quest(id, true))
			}
		}
	}
	if len(vaultedQ) > 0 {
		section := "camp:" + projectID
		expanded := m.collapsedProjects[ui.VaultOpenKey(section)]
		rows = append(rows,
			ui.Row{Kind: ui.RowSpacer},
			ui.Row{Kind: ui.RowVaultHeader, Section: section, Label: fmt.Sprintf("Vaulted (%d)", len(vaultedQ)), Collapsed: !expanded},
		)
		if expanded {
			for _, id := range vaultedQ {
				rows = append(rows, quest(id, true))
			}
		}
	}
	return rows
}

// campState classifies a campaign for the banner overview.
type campState int

const (
	campActive campState = iota
	campLater
	campVaulted
)

// campaignState derives a campaign's status: vaulted when archived, active when
// it has at least one taken-up (active) quest, else later. A completed campaign
// counts as later — it's done with, resting in the hall until you vault it.
func (m *Model) campaignState(p *model.Project) campState {
	if p.Archived {
		return campVaulted
	}
	if !p.IsCompleted() {
		for i := range m.store.Quests {
			if q := &m.store.Quests[i]; q.ProjectID == p.ID && q.Status == model.StatusActive && !q.Vaulted {
				return campActive
			}
		}
	}
	return campLater
}

// bannerPaneRows is a Banner's area overview: loose quests and campaigns split
// into Active / Later / Vaulted, quests listed before campaigns in each group.
// Campaigns show as single rows (Enter/Tab drills into their own view) — the
// banner never lists quests-in-campaigns, only the campaigns themselves. A
// campaign's active/later state is derived from whether it holds active work.
func (m *Model) bannerPaneRows(bannerID string) []ui.Row {
	s := m.store
	real := bannerID != unassignedBanner // the synthetic "Unassigned" holds only campaigns

	header := ui.Row{Kind: ui.RowBanner, BannerID: bannerID, Bare: true}
	if !real {
		header.Label = "Unassigned"
	}
	rows := []ui.Row{header, {Kind: ui.RowSpacer}} // banner title + breathing room below it

	var activeQ, laterQ, vaultedQ []string
	if real {
		for _, q := range s.Quests {
			if q.ProjectID != "" || q.BannerID != bannerID {
				continue
			}
			switch {
			case q.Vaulted:
				vaultedQ = append(vaultedQ, q.ID)
			case q.Status == model.StatusActive:
				activeQ = append(activeQ, q.ID)
			default:
				laterQ = append(laterQ, q.ID)
			}
		}
	}
	var activeC, laterC, vaultedC []string
	for i := range s.Projects {
		p := &s.Projects[i]
		if p.BannerID != bannerID {
			continue
		}
		switch m.campaignState(p) {
		case campVaulted:
			vaultedC = append(vaultedC, p.ID)
		case campActive:
			activeC = append(activeC, p.ID)
		default:
			laterC = append(laterC, p.ID)
		}
	}

	quest := func(id string, dim bool) ui.Row {
		return ui.Row{Kind: ui.RowQuest, QuestID: id, Bare: true, Dim: dim}
	}
	camp := func(id string, dim bool) ui.Row {
		return ui.Row{Kind: ui.RowProject, ProjectID: id, Bare: true, Dim: dim}
	}
	noLooseQuests := real && len(activeQ)+len(laterQ) == 0
	noCampaigns := len(activeC)+len(laterC) == 0

	// One quests-then-campaigns block. A "+ add …" affordance shows only for a
	// kind that's entirely empty; once something's there, Enter adds the next.
	emit := func(qs, cs []string, dim bool) {
		for _, id := range qs {
			rows = append(rows, quest(id, dim))
		}
		if noLooseQuests {
			rows = append(rows, ui.Row{Kind: ui.RowNewQuest, BannerID: bannerID, Label: "+ add quest"})
		}
		if real {
			rows = append(rows, ui.Row{Kind: ui.RowSpacer})
		}
		for _, id := range cs {
			rows = append(rows, camp(id, dim))
		}
		if noCampaigns {
			rows = append(rows, ui.Row{Kind: ui.RowNewProject, BannerID: bannerID, Label: "+ add campaign"})
		}
	}

	if len(activeQ) > 0 || len(activeC) > 0 {
		emit(activeQ, activeC, false)
		// Later: a dimmed backlog under its own divider.
		if len(laterQ)+len(laterC) > 0 {
			rows = append(rows, ui.Row{Kind: ui.RowSpacer}, ui.Row{Kind: ui.RowDayHeader, Label: "Later"}, ui.Row{Kind: ui.RowSpacer})
			for _, id := range laterQ {
				rows = append(rows, quest(id, true))
			}
			if len(laterQ) > 0 && len(laterC) > 0 {
				rows = append(rows, ui.Row{Kind: ui.RowSpacer})
			}
			for _, id := range laterC {
				rows = append(rows, camp(id, true))
			}
		}
	} else {
		// Everything inactive → one flat list, no "Later" split, not dimmed.
		emit(laterQ, laterC, false)
	}

	// Vaulted — collapsed by default.
	if n := len(vaultedQ) + len(vaultedC); n > 0 {
		section := "banner:" + bannerID
		expanded := m.collapsedProjects[ui.VaultOpenKey(section)]
		rows = append(rows,
			ui.Row{Kind: ui.RowSpacer},
			ui.Row{Kind: ui.RowVaultHeader, Section: section, Label: fmt.Sprintf("Vaulted (%d)", n), Collapsed: !expanded},
		)
		if expanded {
			for _, id := range vaultedQ {
				rows = append(rows, ui.Row{Kind: ui.RowQuest, QuestID: id, ShowProjectTag: true})
			}
			for _, id := range vaultedC {
				rows = append(rows, ui.Row{Kind: ui.RowVaultCampaign, ProjectID: id})
			}
		}
	}
	return rows
}

// drillIntoCampaign navigates from a banner overview into one of its campaigns —
// selecting it in the hall and diving into its view.
func (m *Model) drillIntoCampaign(projectID string) {
	m.selectHallTarget(cursorTarget{kind: ui.RowProject, projectID: projectID})
	m.diveIntoPane()
}

// hallRoomLabel is the display name for a room section.
func hallRoomLabel(section string) string {
	switch section {
	case "inbox":
		return "Questboard"
	case "someday":
		return "Vault"
	case "runes":
		return "Runes"
	case "lookouts":
		return "Lookouts"
	}
	return section
}

// ensureHallCursor makes sure the hall selection points at a live entry,
// seeding it (preferring the first area or campaign, else the Questboard) on
// first entry to the Tavern. It only fixes the hall selection — it must NOT
// touch the pane cursor (m.cursor), since it's called from visibleRows() and a
// side effect there would clobber a deliberately-placed cursor.
func (m *Model) ensureHallCursor() {
	rows := m.hallRows()
	if findRowIndex(rows, m.hallCursor) >= 0 {
		return
	}
	// Prefer the first campaign (its pane shows quests to work on), else the
	// first area, else the first selectable entry (the Questboard room).
	for _, want := range []ui.RowKind{ui.RowProject, ui.RowBanner} {
		for _, r := range rows {
			if r.Kind == want {
				m.hallCursor = targetFromRow(r)
				m.hallFocus = true
				return
			}
		}
	}
	if r, ok := nearestSelectableRow(rows, 0); ok {
		m.hallCursor = targetFromRow(r)
		m.hallFocus = true
	}
}

// syncPaneCursor pre-positions the pane cursor on the first row of the current
// selection (so diving in is instant) without stealing focus from the hall.
func (m *Model) syncPaneCursor() {
	m.scrollOffset = 0
	if r, ok := nearestSelectableRow(m.paneRows(), 0); ok {
		m.cursor = targetFromRow(r)
	} else {
		m.cursor = cursorTarget{}
	}
	m.editor = nil
}

// moveHallCursor steps the hall selection, refreshing the pane preview.
func (m *Model) moveHallCursor(delta int) {
	rows := m.hallRows()
	idx := findRowIndex(rows, m.hallCursor)
	if idx < 0 {
		idx = 0
	}
	next := idx
	for {
		next += delta
		if next < 0 {
			next = 0
			break
		}
		if next >= len(rows) {
			next = len(rows) - 1
			break
		}
		if rows[next].Selectable() {
			break
		}
	}
	if !rows[next].Selectable() {
		return
	}
	m.commitEdit()
	m.hallCursor = targetFromRow(rows[next])
	m.cursorMoved = true
	m.syncPaneCursor()
}

// diveIntoPane hands focus to the content pane, landing the cursor on its first
// row.
func (m *Model) diveIntoPane() {
	rows := m.paneRows()
	r, ok := nearestSelectableRow(rows, 0)
	if !ok {
		return
	}
	m.hallFocus = false
	m.scrollOffset = 0
	m.setCursor(r)
}

// returnToHall hands focus back to the hall (committing any pane edit first,
// then dropping an unnamed draft quest/campaign so leaving the pane doesn't
// strand an empty row).
func (m *Model) returnToHall() {
	m.commitEdit()
	m.discardEmptyPaneDraft()
	m.editor = nil
	m.confirmDeleteID = ""
	m.hallFocus = true
	m.cursorMoved = true
}

// selectHallTarget jumps the hall selection to a specific target (used by the
// fuzzy jump and hall clicks), refreshing the pane.
func (m *Model) selectHallTarget(t cursorTarget) {
	rows := m.hallRows()
	if findRowIndex(rows, t) < 0 {
		return
	}
	m.commitEdit()
	m.discardEmptyPaneDraft()
	m.hallCursor = t
	m.hallFocus = true
	m.cursorMoved = true
	m.syncPaneCursor()
}

// renderHallRow renders one hall entry to a single line of the given width.
// selected marks the entry the pane is currently showing; focused is whether
// the hall (vs the pane) currently holds the cursor.
func (m *Model) renderHallRow(row ui.Row, selected, focused bool, width int) string {
	if row.Kind == ui.RowSpacer {
		return ""
	}
	// The cursor "›" shows only while the hall holds focus; a selected-but-
	// unfocused entry stays highlighted (so you can see what the pane is showing)
	// without an active caret.
	mark := "  "
	if selected && focused {
		mark = ui.StyleCursor.Render(ui.GlyphCursor)
	}
	switch row.Kind {
	case ui.RowSection:
		label, count := hallRoomLabel(row.Section), m.hallRoomCount(row.Section)
		motif := sectionMotif(row.Section)
		nameStyle := ui.StyleName
		glyphStyle := ui.StyleMuted
		if selected {
			nameStyle = ui.StyleTitle
			glyphStyle = lipgloss.NewStyle().Foreground(sectionColor(row.Section))
		}
		body := glyphStyle.Render(motif) + " " + nameStyle.Render(label)
		if count > 0 {
			body += ui.StyleMuted.Render(fmt.Sprintf("  %d", count))
		}
		return mark + body
	case ui.RowBanner:
		b := m.findBanner(row.BannerID)
		nm := row.Label
		if b != nil {
			nm = b.Name
		}
		glyph := "\U000f023b" // nf-md-flag
		if b != nil && b.Icon != "" {
			glyph = b.Icon
		}
		count := ui.StyleMuted.Render(fmt.Sprintf(" (%d)", ui.BannerCampaignCount(m.store, row.BannerID)))
		// The synthetic "Unassigned" area reads softer than a real banner.
		if row.BannerID == unassignedBanner {
			nameStyle := ui.StyleMuted
			if selected {
				nameStyle = lipgloss.NewStyle().Bold(true).Foreground(ui.ColorHeading)
			}
			return mark + ui.StyleMuted.Render("○") + " " + nameStyle.Render(strings.ToUpper(nm)) + count
		}
		style := lipgloss.NewStyle().Foreground(ui.ColorHeading)
		if selected {
			style = style.Bold(true)
		}
		return mark + lipgloss.NewStyle().Bold(true).Foreground(ui.ColorHeading).Render(glyph) + " " + style.Render(strings.ToUpper(nm)) + count
	case ui.RowProject:
		p := m.findProject(row.ProjectID)
		if p == nil {
			return ""
		}
		nameStyle := ui.StyleName
		if p.IsCompleted() {
			nameStyle = ui.StyleDone
		}
		if selected {
			nameStyle = ui.StyleTitle
		}
		done, total := ui.ProjectProgress(m.store, p.ID)
		ring := ui.StyleMuted.Render(model.ProgressBucket(done, total))
		indent := "  "
		if row.Nested {
			indent = "    "
		}
		return mark + indent + ring + " " + nameStyle.Render(p.Name)
	case ui.RowNewBanner:
		return mark + ui.StyleMuted.Render("+ New Banner")
	case ui.RowNewProject:
		return mark + ui.StyleMuted.Render("+ New Campaign")
	}
	return mark
}

// activateHallEntry acts on the focused hall entry: a "+ New" affordance
// creates a banner / ungrouped campaign (and dives into the pane to name it);
// anything else just dives into the pane.
func (m *Model) activateHallEntry() tea.Cmd {
	switch m.hallCursor.kind {
	case ui.RowNewBanner:
		b := model.Banner{ID: store.NewID(), Name: ""}
		m.store.Banners = append(m.store.Banners, b)
		m.save()
		m.hallCursor = cursorTarget{kind: ui.RowBanner, bannerID: b.ID}
		m.diveIntoPane() // first pane row is the banner header — edit its name
		return nil
	case ui.RowNewProject:
		p := model.Project{ID: store.NewID(), Name: "", BannerID: m.hallCursor.bannerID}
		m.store.Projects = append(m.store.Projects, p)
		m.save()
		m.hallCursor = cursorTarget{kind: ui.RowProject, projectID: p.ID}
		m.diveIntoPane()
		return nil
	}
	m.diveIntoPane()
	return nil
}

// hallRoomCount is the count badge shown beside a room in the hall.
func (m *Model) hallRoomCount(section string) int {
	switch section {
	case "inbox":
		return ui.CountInbox(m.store)
	case "someday":
		return ui.CountSomeday(m.store) + ui.CountArchived(m.store)
	case "runes":
		return ui.CountRunes(m.store)
	case "lookouts":
		return ui.CountLookouts(m.store)
	}
	return 0
}

// hallColWidth is the fixed width of the left directory column.
func hallColWidth(contentW int) int { return clampInt(contentW/3, 18, 30) }

// hallGap is the breathing room between the left hall and the content pane.
const hallGap = 6

// renderTavernView draws the borderless hall + content pane. The header (mode
// toggle + greeting) is pinned to a fixed top; below it the two columns float
// side by side, each scrolling to follow its own cursor.
func (m *Model) renderTavernView() string {
	m.ensureHallCursor()

	contentW := clampInt(m.width-4, 40, 100)
	outer := (m.width - contentW) / 2
	if outer < 0 {
		outer = 0
	}
	m.leftMargin = outer
	margin := strings.Repeat(" ", outer)

	hallW := hallColWidth(contentW)
	paneW := contentW - hallW - hallGap
	if paneW < 20 {
		paneW = 20
	}
	m.hallColX, m.hallColW = outer, hallW
	m.paneColX, m.paneColW = outer+hallW+hallGap, paneW

	header := []string{
		m.renderModeToggle(contentW),
		ui.CenterText(ui.StyleMuted.Render(m.subtitle), contentW),
	}
	footer := indentLines(m.statusBar(contentW), margin)
	hallRows := m.hallRows()
	paneRows := m.paneRows()
	contentRows := len(hallRows)
	if len(paneRows) > contentRows {
		contentRows = len(paneRows)
	}
	// Vertical layout: a comfortable, roughly equal margin top and bottom; the
	// body is a capped-height scrolling viewport, and the footer sits just below
	// it (right after the items) rather than pinned to the screen edge.
	const bodyGap, footerGap = 3, 2
	comfy := clampInt(m.height/8, 2, 5)
	chrome := len(header) + bodyGap + footerGap + lipgloss.Height(footer)
	colBodyH := contentRows
	if maxBody := m.height - chrome - 2*comfy; colBodyH > maxBody {
		colBodyH = maxBody
	}
	if colBodyH < 1 {
		colBodyH = 1
	}
	topPad := (m.height - chrome - colBodyH) / 2
	if topPad < comfy {
		topPad = comfy
	}
	if topPad < 0 {
		topPad = 0
	}
	colTop := topPad + len(header) + bodyGap

	hallIdx := findRowIndex(hallRows, m.hallCursor)
	paneIdx := -1
	if !m.hallFocus {
		paneIdx = findRowIndex(paneRows, m.cursor)
	}

	m.hallScroll = followScroll(len(hallRows), hallIdx, m.hallScroll, colBodyH, m.hallFocus && m.cursorMoved)
	m.scrollOffset = followScroll(len(paneRows), paneIdx, m.scrollOffset, colBodyH, !m.hallFocus && m.cursorMoved)
	// Cache each column's max scroll so the wheel can move a column even when it
	// doesn't hold the cursor (e.g. peeking a long pane while focus is in the hall).
	m.hallScrollMax = maxInt(0, len(hallRows)-colBodyH)
	m.scrollMax = maxInt(0, len(paneRows)-colBodyH)

	m.rowsScreenTop = colTop
	m.hallSpans = m.hallSpans[:0]
	m.hintSpans = map[int][]hintSpan{}
	m.codeSpans = map[int][]codeSpan{}

	hoverIdx := -1
	if m.hover != nil && !m.hallFocus {
		hoverIdx = findRowIndex(paneRows, *m.hover)
	}

	// Record the cursor's screen cell for overlay bursts (pane side only).
	m.cursorScreenY = -1
	if !m.hallFocus && paneIdx >= m.scrollOffset && paneIdx < m.scrollOffset+colBodyH {
		m.cursorScreenY = colTop + (paneIdx - m.scrollOffset)
		m.cursorScreenX = m.paneColX
	}

	clip := lipgloss.NewStyle().MaxWidth(m.width)
	var b strings.Builder
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}
	for _, line := range header {
		b.WriteString(clip.Render(margin+line) + "\n")
	}
	b.WriteString(strings.Repeat("\n", bodyGap))
	m.modeToggleRow = topPad
	m.tavernHelpRow = topPad

	for i := 0; i < colBodyH; i++ {
		hallLine := m.hallLineAt(hallRows, m.hallScroll, i, colBodyH, hallW, hallIdx, colTop)
		paneLine := m.paneLineAt(paneRows, i, colBodyH, paneW, paneIdx, hoverIdx)
		b.WriteString(clip.Render(margin+fitWidth(hallLine, hallW)+strings.Repeat(" ", hallGap)+paneLine) + "\n")
	}
	// The footer sits just below the body (padToScreen fills the balanced bottom
	// margin) — static, appearing at once like Camp's rather than animating in.
	b.WriteString(strings.Repeat("\n", footerGap))
	b.WriteString(footer)

	m.cursorMoved = false
	return b.String()
}

// currentBodyAbsolute renders the current view's body to lines with the left
// margin baked in (absolute) — captured at the start of a Camp⇄Tavern switch so
// the departing view can dissolve away in place (see renderModeDissolve).
func (m *Model) currentBodyAbsolute() []string {
	if m.inTavern() {
		return m.tavernBodyAbsolute()
	}
	cw := m.contentWidth()
	margin := strings.Repeat(" ", max0((m.width-cw)/2))
	var out []string
	for _, r := range m.visibleRows() {
		if r.Kind == ui.RowQuestMeta {
			continue
		}
		line, _ := ui.RenderRow(r, m.store, "", false, false, cw, "")
		out = append(out, margin+line)
	}
	return out
}

// tavernBodyAbsolute renders the hall+pane body to absolute (margin-baked) lines
// for the dissolve capture.
func (m *Model) tavernBodyAbsolute() []string {
	contentW := clampInt(m.width-4, 40, 100)
	margin := strings.Repeat(" ", max0((m.width-contentW)/2))
	hallW := hallColWidth(contentW)
	paneW := contentW - hallW - hallGap
	if paneW < 20 {
		paneW = 20
	}
	hallRows := m.hallRows()
	paneRows := m.paneRows()
	n := len(hallRows)
	if len(paneRows) > n {
		n = len(paneRows)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hallLine := ""
		if i < len(hallRows) {
			hallLine = m.renderHallRow(hallRows[i], false, false, hallW)
		}
		paneLine := ""
		if i < len(paneRows) && paneRows[i].Kind != ui.RowSpacer {
			paneLine, _ = ui.RenderRow(paneRows[i], m.store, "", false, false, paneW, "")
		}
		out = append(out, margin+fitWidth(hallLine, hallW)+strings.Repeat(" ", hallGap)+paneLine)
	}
	return out
}

// hallLineAt renders the hall column's line i (with fold markers), recording a
// click span for a selectable entry.
func (m *Model) hallLineAt(rows []ui.Row, scroll, i, viewH, width, cursorIdx, colTop int) string {
	if i == 0 && scroll > 0 {
		return ui.StyleMuted.Render(ellipsis)
	}
	if i == viewH-1 && scroll+viewH < len(rows) {
		return ui.StyleMuted.Render(ellipsis)
	}
	src := scroll + i
	if src < 0 || src >= len(rows) {
		return ""
	}
	row := rows[src]
	if row.Selectable() {
		m.hallSpans = append(m.hallSpans, hallSpan{y: colTop + i, target: targetFromRow(row)})
	}
	return m.renderHallRow(row, src == cursorIdx, m.hallFocus, width)
}

// paneLineAt renders the pane column's line i (with fold markers).
func (m *Model) paneLineAt(rows []ui.Row, i, viewH, width, cursorIdx, hoverIdx int) string {
	if i == 0 && m.scrollOffset > 0 {
		return ui.StyleMuted.Render(ellipsis)
	}
	if i == viewH-1 && m.scrollOffset+viewH < len(rows) {
		return ui.StyleMuted.Render(ellipsis)
	}
	src := m.scrollOffset + i
	if src < 0 || src >= len(rows) {
		return ""
	}
	if rows[src].Kind == ui.RowSpacer {
		return ""
	}
	return m.renderOutlineRowLine(rows, src, cursorIdx, hoverIdx, -1, width, m.paneColX)
}

// followScroll clamps a column's scroll so cursorIdx stays visible (when
// following) and never runs past the content's end.
func followScroll(total, cursorIdx, scroll, viewH int, follow bool) int {
	if follow && cursorIdx >= 0 {
		if cursorIdx < scroll {
			scroll = cursorIdx
		}
		if cursorIdx >= scroll+viewH {
			scroll = cursorIdx - viewH + 1
		}
	}
	if max := total - viewH; scroll > max {
		scroll = max
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

// handleTavernClick routes a left-click in the Tavern to the mode toggle, a hall
// entry, or a pane row.
func (m *Model) handleTavernClick(msg tea.MouseClickMsg) tea.Cmd {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return nil
	}
	m.confirmDeleteID = ""
	// The header row: mode toggle (TAVERN/CAMP) + right-aligned F1 help.
	if mouse.Y == m.modeToggleRow {
		if m.tavernHelpWidth > 0 && mouse.X >= m.tavernHelpX && mouse.X < m.tavernHelpX+m.tavernHelpWidth {
			m.pushModal(helpModal())
			return nil
		}
		for _, sp := range m.modeSpans {
			if mouse.X >= sp.x0 && mouse.X < sp.x1 {
				return m.setWilds(sp.wilds)
			}
		}
		return nil
	}
	// Hall column: a click selects that entry.
	if mouse.X >= m.hallColX && mouse.X < m.hallColX+m.hallColW {
		for _, sp := range m.hallSpans {
			if sp.y == mouse.Y {
				m.selectHallTarget(sp.target)
				if sp.target.kind == ui.RowNewBanner || sp.target.kind == ui.RowNewProject {
					return m.activateHallEntry() // clicking a "+ New" creates + dives in
				}
				return nil
			}
		}
		return nil
	}
	// Pane column: focus the pane and act on the clicked row.
	if mouse.X >= m.paneColX {
		rows := m.paneRows()
		relY := mouse.Y - m.rowsScreenTop
		if relY < 0 {
			return nil
		}
		idx := m.scrollOffset + relY
		if idx < 0 || idx >= len(rows) {
			return nil
		}
		m.hallFocus = false
		return m.clickRowAt(rows, idx, mouse, m.paneColX)
	}
	return nil
}

// handleTavernWheel scrolls whichever column the pointer is over.
func (m *Model) handleTavernWheel(msg tea.MouseWheelMsg) tea.Cmd {
	mouse := msg.Mouse()
	m.confirmDeleteID = ""
	delta := 1
	if mouse.Button == tea.MouseWheelUp {
		delta = -1
	}
	if mouse.X >= m.hallColX && mouse.X < m.hallColX+m.hallColW {
		m.hallScroll = clampInt(m.hallScroll+delta, 0, m.hallScrollMax)
		m.invalidateRender()
		return nil
	}
	m.scrollOffset = clampInt(m.scrollOffset+delta, 0, m.scrollMax)
	m.invalidateRender()
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
