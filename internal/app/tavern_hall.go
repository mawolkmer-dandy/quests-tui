package app

import (
	"fmt"
	"sort"
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

// paneLinkSpan is a shortened-link click target in a campaign note: vis is its
// pane visual-line index; [x0,x1) are absolute screen columns; url is the full
// address. Checked before the row click so a link click copies/opens it.
type paneLinkSpan struct {
	vis    int
	x0, x1 int
	url    string
}

// unassignedBanner is the sentinel BannerID for the "Unassigned" area that
// gathers campaigns not filed under any real banner.
const unassignedBanner = ""

// errandsBanner is the reserved BannerID of the pinned "Errands" area — the
// permanent home for campaign-less quests (one-off errands + recurring rites).
// Always rendered first among the areas. It isn't stored in Store.Banners, so
// findBanner returns nil for it and, like the synthetic "Unassigned", it can't
// be renamed, reordered, or deleted. Shares ui.ErrandsBannerID so rendering can
// give it its distinct treatment.
const errandsBanner = ui.ErrandsBannerID

// errandsLabel is the display name of the pinned Errands area.
const errandsLabel = "Errands"

// hallRows is the left directory: the rooms group on top (Questboard · Vault ·
// Runes · Lookouts, the latter two only when non-empty), then the areas —
// each Banner with its campaigns nested beneath, ungrouped campaigns gathered
// under an "Unassigned" area, and the "+ New" affordances at the base. Groups
// are separated by blank rows (no rules) for a calmer, roomier hall.
func (m *Model) hallRows() []ui.Row {
	s := m.store
	blank := ui.Row{Kind: ui.RowSpacer}
	rows := []ui.Row{{Kind: ui.RowSection, Section: "inbox"}}
	if m.hasAnyTrails() {
		rows = append(rows, ui.Row{Kind: ui.RowSection, Section: "trails"})
	}
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
	// Errands: the pinned home area for campaign-less quests — always first among
	// the areas (above real banners, below the rooms).
	appendArea(errandsBanner, errandsLabel)
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

	// Inline notes (Project.Body) sit directly under the title — plain prose,
	// edited with the shared body outline editor. An empty campaign shows a
	// single "add notes…" placeholder (RowBodyLine with no BodyLineID).
	if p := m.findProject(projectID); p != nil {
		if len(p.Body) == 0 {
			rows = append(rows, ui.Row{Kind: ui.RowBodyLine, ProjectID: projectID})
		} else {
			for _, l := range p.Body {
				rows = append(rows, ui.Row{Kind: ui.RowBodyLine, ProjectID: projectID, BodyLineID: l.ID})
			}
		}
		rows = append(rows, ui.Row{Kind: ui.RowSpacer})
	}

	var activeQ, laterQ, doneQ, vaultedQ []string
	for _, q := range ui.QuestsForCampaign(s, projectID) { // sorted, excludes vaulted
		switch {
		case q.Status == model.StatusActive:
			activeQ = append(activeQ, q.ID)
		case q.Status == model.StatusDone:
			doneQ = append(doneQ, q.ID)
		default:
			laterQ = append(laterQ, q.ID) // open backlog
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
	group := func(label string, ids []string) {
		if len(ids) == 0 {
			return
		}
		rows = append(rows, ui.Row{Kind: ui.RowSpacer}, ui.Row{Kind: ui.RowDayHeader, Label: label}, ui.Row{Kind: ui.RowSpacer})
		for _, id := range ids {
			rows = append(rows, quest(id, true))
		}
	}

	switch {
	case len(activeQ) == 0 && len(laterQ) == 0:
		// No active or backlog work → offer a new quest (done ones show below).
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
		group("Later", laterQ)
	}
	// Done — completed quests, resting until vaulted, in their own group.
	group("Done", doneQ)
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

	// Saga chapter links, pinned at the bottom: the previous chapter (derived)
	// and the next (Project.NextID), each drilling into that campaign.
	var saga []ui.Row
	if prev := m.prevChapter(projectID); prev != nil {
		saga = append(saga, ui.Row{Kind: ui.RowSagaLink, ProjectID: prev.ID, Label: "← Continued from: "})
	}
	if p := m.findProject(projectID); p != nil && p.NextID != "" {
		if next := m.findProject(p.NextID); next != nil {
			saga = append(saga, ui.Row{Kind: ui.RowSagaLink, ProjectID: next.ID, Label: "→ Continues in: "})
		}
	}
	if len(saga) > 0 {
		rows = append(rows, ui.Row{Kind: ui.RowSpacer})
		rows = append(rows, saga...)
	}

	return rows
}

// campState classifies a campaign for the banner overview.
type campState int

const (
	campActive campState = iota
	campLater
	campDone
	campVaulted
)

// campaignState derives a campaign's status: vaulted when archived, done when
// completed (resting in the hall until you vault it), active when it has at
// least one taken-up (active) quest, else later (open backlog).
func (m *Model) campaignState(p *model.Project) campState {
	if p.Archived {
		return campVaulted
	}
	if p.IsCompleted() {
		return campDone
	}
	for i := range m.store.Quests {
		if q := &m.store.Quests[i]; q.ProjectID == p.ID && q.Status == model.StatusActive && !q.Vaulted {
			return campActive
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
	switch bannerID {
	case unassignedBanner:
		header.Label = "Unassigned"
	case errandsBanner:
		header.Label = errandsLabel // synthetic — findBanner can't supply the name
	}
	rows := []ui.Row{header, {Kind: ui.RowSpacer}} // banner title + breathing room below it

	var activeQ, laterQ, doneQ, vaultedQ []string
	if real {
		// Collect the loose quests and order them by priority (like a campaign's
		// pane does) — else low-priority quests would sit in raw store order,
		// above no-priority ones.
		var loose []model.Quest
		for _, q := range s.Quests {
			if q.ProjectID == "" && q.BannerID == bannerID {
				loose = append(loose, q)
			}
		}
		sort.SliceStable(loose, func(i, j int) bool { return ui.SortBucket(loose[i]) < ui.SortBucket(loose[j]) })
		for _, q := range loose {
			switch {
			case q.Vaulted:
				vaultedQ = append(vaultedQ, q.ID)
			case q.Status == model.StatusActive:
				activeQ = append(activeQ, q.ID)
			case q.Status == model.StatusDone:
				doneQ = append(doneQ, q.ID)
			default:
				laterQ = append(laterQ, q.ID)
			}
		}
	}
	var activeC, laterC, doneC, vaultedC []string
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
		case campDone:
			doneC = append(doneC, p.ID)
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
	// The pinned Errands area holds only loose quests — it never lists or offers
	// to add campaigns.
	errands := bannerID == errandsBanner
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
		if errands {
			return // no campaigns under Errands
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

	// group renders a labeled, dimmed block (Later / Done) under its own divider —
	// quests first, then campaigns, a blank line between when both are present.
	group := func(label string, qs, cs []string) {
		if len(qs)+len(cs) == 0 {
			return
		}
		rows = append(rows, ui.Row{Kind: ui.RowSpacer}, ui.Row{Kind: ui.RowDayHeader, Label: label}, ui.Row{Kind: ui.RowSpacer})
		for _, id := range qs {
			rows = append(rows, quest(id, true))
		}
		if len(qs) > 0 && len(cs) > 0 {
			rows = append(rows, ui.Row{Kind: ui.RowSpacer})
		}
		for _, id := range cs {
			rows = append(rows, camp(id, true))
		}
	}

	if len(activeQ) > 0 || len(activeC) > 0 {
		emit(activeQ, activeC, false)
		group("Later", laterQ, laterC)
	} else {
		// Nothing taken up → show the backlog flat (no "Later" header), or just the
		// "+ add" affordances when it's empty.
		emit(laterQ, laterC, false)
	}
	// Done — completed campaigns/quests, resting until vaulted, in their own group.
	group("Done", doneQ, doneC)

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

// navigateToQuest jumps the hall to a quest's container (campaign / area /
// Questboard), dives into the pane, and lands the cursor on the quest itself —
// the "go to this quest" the search modal fires when a quest is chosen.
func (m *Model) navigateToQuest(questID string) {
	q := m.findQuest(questID)
	if q == nil {
		return
	}
	var container cursorTarget
	switch {
	case q.ProjectID != "":
		container = cursorTarget{kind: ui.RowProject, projectID: q.ProjectID}
	case q.BannerID != "":
		container = cursorTarget{kind: ui.RowBanner, bannerID: q.BannerID}
	default:
		container = cursorTarget{kind: ui.RowSection, section: "inbox"}
	}
	m.selectHallTarget(container)
	for _, r := range m.paneRows() {
		if r.Kind == ui.RowQuest && r.QuestID == questID {
			m.hallFocus = false
			m.scrollOffset = 0
			m.setCursor(r)
			return
		}
	}
	m.diveIntoPane() // fallback: the quest row wasn't found — land on the first row
}

// escFromPane handles Esc while focused in the Tavern pane. Inside a campaign it
// steps UP to that campaign's banner view first, so Esc walks the hierarchy
// campaign → banner → sidebar; a second Esc from the banner then hands focus to
// the hall. Anywhere else (a banner, a room) Esc goes straight to the hall.
func (m *Model) escFromPane() {
	if m.hallCursor.kind == ui.RowProject {
		if p := m.findProject(m.hallCursor.projectID); p != nil {
			target := cursorTarget{kind: ui.RowBanner, bannerID: p.BannerID}
			if findRowIndex(m.hallRows(), target) >= 0 {
				m.selectHallTarget(target)
				m.diveIntoPane()
				return
			}
		}
	}
	m.returnToHall()
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
	case "trails":
		return "Trails"
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
	fromNote := ""
	if m.cursor.kind == ui.RowBodyLine {
		fromNote = m.cursor.projectID
	}
	m.commitEdit()
	m.discardEmptyPaneDraft()
	if fromNote != "" {
		m.pruneTrailingEmptyNotes(fromNote)
	}
	m.editor = nil
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
		count := ui.StyleMuted.Render(fmt.Sprintf(" (%d)", ui.BannerItemCount(m.store, row.BannerID)))
		// The synthetic "Unassigned" area reads softer than a real banner.
		if row.BannerID == unassignedBanner {
			nameStyle := ui.StyleMuted
			if selected {
				nameStyle = lipgloss.NewStyle().Bold(true).Foreground(ui.ColorHeading)
			}
			return mark + ui.StyleMuted.Render("○") + " " + nameStyle.Render(strings.ToUpper(nm)) + count
		}
		// The pinned Errands area gets the tavern's warm accent and a checklist
		// emblem, so it reads as a fixture rather than one of your own areas.
		if row.BannerID == errandsBanner {
			nameStyle := lipgloss.NewStyle().Foreground(ui.ColorAccent)
			if selected {
				nameStyle = nameStyle.Bold(true)
			}
			return mark + lipgloss.NewStyle().Bold(true).Foreground(ui.ColorAccent).Render(ui.GlyphErrands) + " " + nameStyle.Render(strings.ToUpper(nm)) + count
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
		return mark + indent + ui.PriorityIndicator(p.Priority) + ring + " " + nameStyle.Render(p.Name)
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
	case "trails":
		return m.trailsCount()
	}
	return 0
}

// hallColWidth is the fixed width of the left directory column.
func hallColWidth(contentW int) int { return clampInt(contentW/3, 18, 36) }

// hallGap is the breathing room between the left hall and the content pane.
const hallGap = 6

// renderTavernView draws the borderless hall + content pane. The header (mode
// toggle + greeting) is pinned to a fixed top; below it the two columns float
// side by side, each scrolling to follow its own cursor.
func (m *Model) renderTavernView() string {
	m.ensureHallCursor()

	contentW := clampInt(m.width-4, 40, 130)
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

	hallIdx := findRowIndex(hallRows, m.hallCursor)
	paneIdx := -1
	if !m.hallFocus {
		paneIdx = findRowIndex(paneRows, m.cursor)
	}
	hoverIdx := -1
	if m.hover != nil && !m.hallFocus {
		hoverIdx = findRowIndex(paneRows, *m.hover)
	}

	// Reset the per-render click maps before rendering rows records into them.
	m.hallSpans = m.hallSpans[:0]
	m.hintSpans = map[int][]hintSpan{}
	m.codeSpans = map[int][]codeSpan{}
	m.paneLinkSpans = m.paneLinkSpans[:0]

	// Pane rows expand to screen lines (a campaign note soft-wraps its prose);
	// the hall stays one row per line. The two columns scroll independently.
	paneLines, paneLineRow, paneRowFirst := m.expandedPaneLines(paneRows, paneW, paneIdx, hoverIdx)
	m.paneLineRow = paneLineRow

	// Vertical layout: a comfortable, roughly equal margin top and bottom. The
	// body fills the available height so the hall doesn't scroll before it must
	// and the tall Vault has room — but it's still a FIXED height (depends only on
	// the window, not the selected page), so the Tavern keeps one size instead of
	// snapping between a short page (an errand) and a tall one (the Vault). Short
	// pages pad with blank space; taller content scrolls within it.
	const bodyGap, footerGap = 3, 2
	comfy := clampInt(m.height/8, 2, 4)
	chrome := len(header) + bodyGap + footerGap + lipgloss.Height(footer)
	colBodyH := m.height - chrome - 2*comfy
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

	// The cursor's first screen line (a wrapped note occupies several).
	paneVisIdx := -1
	if paneIdx >= 0 {
		paneVisIdx = paneRowFirst[paneIdx]
	}
	m.hallScroll = followScroll(len(hallRows), hallIdx, m.hallScroll, colBodyH, m.hallFocus && m.cursorMoved)
	m.scrollOffset = followScroll(len(paneLines), paneVisIdx, m.scrollOffset, colBodyH, !m.hallFocus && m.cursorMoved)
	// Cache each column's max scroll so the wheel can move a column even when it
	// doesn't hold the cursor (e.g. peeking a long pane while focus is in the hall).
	m.hallScrollMax = maxInt(0, len(hallRows)-colBodyH)
	m.scrollMax = maxInt(0, len(paneLines)-colBodyH)

	m.rowsScreenTop = colTop

	// Record the cursor's screen cell for overlay bursts (pane side only).
	m.cursorScreenY = -1
	if !m.hallFocus && paneVisIdx >= m.scrollOffset && paneVisIdx < m.scrollOffset+colBodyH {
		m.cursorScreenY = colTop + (paneVisIdx - m.scrollOffset)
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
		paneLine := m.paneVisLineAt(paneLines, i, colBodyH)
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
	contentW := clampInt(m.width-4, 40, 130)
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

// expandedPaneLines renders paneRows to a flat list of screen lines — every row
// is exactly one line except a RowBodyLine (a campaign note), whose prose
// soft-wraps across as many lines as it needs. lineRow[i] is the paneRows index
// screen line i belongs to (-1 for a spacer's blank line); rowFirst[j] is the
// first screen line of row j, so scroll/cursor/click can translate between rows
// and screen lines. Recording of clickable spans (via renderOutlineRowLine)
// stays keyed by row index, so it's unaffected by the wrapping.
func (m *Model) expandedPaneLines(rows []ui.Row, width, cursorIdx, hoverIdx int) (lines []string, lineRow, rowFirst []int) {
	rowFirst = make([]int, len(rows))
	for j, r := range rows {
		rowFirst[j] = len(lines)
		switch r.Kind {
		case ui.RowSpacer:
			lines = append(lines, "")
			lineRow = append(lineRow, -1)
		case ui.RowBodyLine:
			base := len(lines)
			segLines, runs := m.renderNoteScreenLines(r, width)
			for _, seg := range segLines {
				lines = append(lines, seg)
				lineRow = append(lineRow, j)
			}
			for _, run := range runs {
				m.paneLinkSpans = append(m.paneLinkSpans, paneLinkSpan{
					vis: base + run.seg, x0: m.paneColX + run.x0, x1: m.paneColX + run.x1, url: run.url,
				})
			}
		default:
			lines = append(lines, m.renderOutlineRowLine(rows, j, cursorIdx, hoverIdx, -1, width, m.paneColX))
			lineRow = append(lineRow, j)
		}
	}
	return lines, lineRow, rowFirst
}

// paneVisLineAt returns pane screen line i (post soft-wrap), with a scroll
// ellipsis at the first/last visible line when there's more content off-screen.
func (m *Model) paneVisLineAt(lines []string, i, viewH int) string {
	if i == 0 && m.scrollOffset > 0 {
		return ui.StyleMuted.Render(ellipsis)
	}
	if i == viewH-1 && m.scrollOffset+viewH < len(lines) {
		return ui.StyleMuted.Render(ellipsis)
	}
	idx := m.scrollOffset + i
	if idx < 0 || idx >= len(lines) {
		return ""
	}
	return lines[idx]
}

// followScroll clamps a column's scroll so cursorIdx stays visible (when
// following) and never runs past the content's end.
func followScroll(total, cursorIdx, scroll, viewH int, follow bool) int {
	if follow && cursorIdx >= 0 {
		// Keep a one-line margin at each edge: when scrolled, the first/last
		// visible line is replaced by a "…" more-indicator, so parking the cursor
		// on that line would hide the very row you're on (this stranded the top
		// row of a scrolled Vault behind the "…"). The end-clamps below release the
		// margin at the true top/bottom, where no ellipsis is drawn.
		margin := 1
		if viewH <= 2 {
			margin = 0
		}
		if cursorIdx < scroll+margin {
			scroll = cursorIdx - margin
		}
		if cursorIdx > scroll+viewH-1-margin {
			scroll = cursorIdx - viewH + 1 + margin
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
	// Pane column: focus the pane and act on the clicked row. The click lands on
	// a screen line, which maps back to its row via paneLineRow (a wrapped note
	// spans several lines; a spacer's blank line maps to -1).
	if mouse.X >= m.paneColX {
		relY := mouse.Y - m.rowsScreenTop
		if relY < 0 {
			return nil
		}
		visIdx := m.scrollOffset + relY
		if visIdx < 0 || visIdx >= len(m.paneLineRow) {
			return nil
		}
		// A click on a shortened link in a note copies it (a fast second opens),
		// the same gesture as a quest body link — checked before the row click.
		for _, sp := range m.paneLinkSpans {
			if sp.vis == visIdx && mouse.X >= sp.x0 && mouse.X < sp.x1 {
				m.hallFocus = false
				return m.clickLink(sp.url)
			}
		}
		idx := m.paneLineRow[visIdx]
		if idx < 0 {
			return nil
		}
		m.hallFocus = false
		return m.clickRowAt(m.paneRows(), idx, mouse, m.paneColX)
	}
	return nil
}

// handleTavernWheel scrolls whichever column the pointer is over.
func (m *Model) handleTavernWheel(msg tea.MouseWheelMsg) tea.Cmd {
	mouse := msg.Mouse()
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
