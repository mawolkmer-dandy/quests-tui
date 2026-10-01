package app

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	// An inline Lookout rename owns every key while it's open (same as in the
	// detail view — see handleLookoutRenameKey).
	if cmd, handled := m.handleLookoutRenameKey(msg); handled {
		return cmd
	}
	switch {
	case key.Matches(msg, Keys.Help):
		m.commitEdit()
		m.pushModal(helpModal())
		return nil
	// Ctrl+F is Search — the unified fuzzy finder over rooms, banners, campaigns,
	// and quests (replaces the old inline filter bar and the jump modal).
	case m.inTavern() && key.Matches(msg, Keys.Search):
		m.commitEdit()
		m.pushModal(searchModal(m.store))
		return nil
	case key.Matches(msg, Keys.ToggleHints):
		m.hideHoverTips = !m.hideHoverTips
		label := "hover tips shown"
		if m.hideHoverTips {
			label = "hover tips hidden"
		}
		return m.showClipboardToastText(label) // top-right toast
	case key.Matches(msg, Keys.Undo):
		m.undo()
		return nil
	case key.Matches(msg, Keys.SetOut):
		if m.venturing() {
			// From deep in the Wilds, Ctrl+G heads all the way back to the Tavern
			// (the session is logged first — bailing is never penalized).
			m.logVentureSession(false)
			m.endVenture()
			return m.setWilds(false)
		}
		return m.setWilds(!m.wilds)
	case msg.Code == tea.KeyEsc && m.venturing():
		return m.makeCamp(false) // step back out of the Wilds to Camp
	case msg.Code == tea.KeyEsc && m.wilds:
		return m.setWilds(false)
	// Ctrl+L links the current campaign to its next saga chapter.
	case m.inTavern() && msg.String() == "ctrl+l":
		return m.openSagaPicker()
	// Ctrl+1 / Ctrl+2 move focus between the sidebar (hall) and the content pane.
	case m.inTavern() && msg.String() == "ctrl+1":
		m.returnToHall()
		return nil
	case m.inTavern() && msg.String() == "ctrl+2":
		m.diveIntoPane()
		return nil
	// Ctrl+N is "new": a Banner from the hall, a contextual sibling in the pane.
	case m.inTavern() && msg.String() == "ctrl+n":
		if m.hallFocus {
			return m.newBannerFromHall()
		}
		return m.contextualCreate()
	// Tavern hall (master-detail): ↑↓ move the selection, Tab/→ dive into the
	// pane (navigation), Enter adds a campaign in the current area.
	case m.inTavern() && m.hallFocus && key.Matches(msg, Keys.Up):
		m.moveHallCursor(-1)
		return nil
	case m.inTavern() && m.hallFocus && key.Matches(msg, Keys.Down):
		m.moveHallCursor(1)
		return nil
	// Shift+↑/↓ reorders the selected banner in the sidebar (like reordering
	// quests/campaigns in the pane).
	case m.inTavern() && m.hallFocus && m.hallCursor.kind == ui.RowBanner && key.Matches(msg, Keys.MoveUp):
		m.moveBanner(-1)
		return nil
	case m.inTavern() && m.hallFocus && m.hallCursor.kind == ui.RowBanner && key.Matches(msg, Keys.MoveDown):
		m.moveBanner(1)
		return nil
	// Ctrl+P cycles a campaign's priority from the hall (it works on a quest in
	// the pane via handleRowKey; the hall selection isn't the pane cursor).
	case m.inTavern() && m.hallFocus && m.hallCursor.kind == ui.RowProject && key.Matches(msg, Keys.ToggleImportant):
		m.cycleProjectPriority(m.hallCursor.projectID)
		return nil
	case m.inTavern() && m.hallFocus && (key.Matches(msg, Keys.Tab) || key.Matches(msg, Keys.Right)):
		m.diveIntoPane()
		return nil
	case m.inTavern() && m.hallFocus && key.Matches(msg, Keys.Enter):
		return m.hallCreate()
	// Ctrl+X in the hall confirms deleting whatever's selected there — a banner
	// (its campaigns/loose quests survive, ungrouped) or a campaign — acting on
	// the hall selection, not the pane cursor beneath it.
	case m.inTavern() && m.hallFocus && key.Matches(msg, Keys.Delete):
		m.openDeleteModal(m.hallCursor)
		return nil
	case m.inTavern() && !m.hallFocus && msg.Code == tea.KeyEsc:
		m.escFromPane()
		return nil
	case key.Matches(msg, Keys.Up):
		m.moveCursor(-1)
		return nil
	case key.Matches(msg, Keys.Down):
		m.moveCursor(1)
		return nil
	case msg.Code == tea.KeyPgUp, msg.Code == tea.KeyPgDown:
		half := m.height / 2
		if half < 1 {
			half = 1
		}
		delta := 1
		if msg.Code == tea.KeyPgUp {
			delta = -1
		}
		step := m.moveCursor
		if m.inTavern() && m.hallFocus {
			step = m.moveHallCursor
		}
		for i := 0; i < half; i++ {
			step(delta)
		}
		return nil
	}
	return m.handleRowKey(msg)
}

// handleRowKey handles every action keyed off whatever m.cursor currently
// targets — shared by the Tavern pane and a focused section page, so
// Tab/Enter/Ctrl+D/Ctrl+X/etc. behave identically in both places.
func (m *Model) handleRowKey(msg tea.KeyPressMsg) tea.Cmd {
	// A campaign notes line intercepts its editing keys (split/merge, typing,
	// copy-paste) for the shared body editor; navigation keys fall through to
	// the ordinary pane cursor below.
	if m.cursor.kind == ui.RowBodyLine {
		if cmd, handled := m.handleCampaignNoteEditKey(msg); handled {
			return cmd
		}
	}

	switch {
	case key.Matches(msg, Keys.MoveUp):
		m.moveRow(-1)
		return nil
	case key.Matches(msg, Keys.MoveDown):
		m.moveRow(1)
		return nil
	case key.Matches(msg, Keys.Tab):
		return m.handleReveal()
	case key.Matches(msg, Keys.Enter):
		return m.handleEnter()
	case msg.Code == tea.KeyBackspace:
		return m.handleBackspace(msg)
	case key.Matches(msg, Keys.ToggleActive):
		return m.toggleActive()
	case key.Matches(msg, Keys.ToggleDone):
		return m.toggleDone()
	case key.Matches(msg, Keys.ToggleImportant):
		return m.cyclePriority()
	case key.Matches(msg, Keys.ToggleVault):
		m.toggleVault()
		return nil
	case key.Matches(msg, Keys.ToggleType):
		m.toggleType()
		return nil
	case key.Matches(msg, Keys.MoveProject):
		m.openProjectPicker()
		return nil
	case key.Matches(msg, Keys.Schedule):
		return m.openSchedulePicker()
	case key.Matches(msg, Keys.Delete):
		m.openDeleteModal(m.cursor)
		return nil
	case key.Matches(msg, Keys.Find):
		if m.cursor.kind == ui.RowQuest {
			return m.findTracksInTrails(m.cursor.questID)
		}
		return nil
	case m.cursor.kind == ui.RowLookout && (msg.String() == "r" || key.Matches(msg, Keys.Rename)):
		// Rename a dashboard from the Tavern too — same inline editor as the
		// detail view (its name can't be scraped from the URL).
		m.beginLookoutRename(m.cursor.questID, m.cursor.lookoutURL)
		return nil
	case msg.String() == "c" && m.cursor.kind == ui.RowTrack:
		return m.copyTrack(m.cursor.questID, m.cursor.trackEvent)
	case msg.String() == "c" && m.cursor.kind == ui.RowRune:
		return m.copyToClipboard(ldFlagURL(m.ldProject, m.ldEnv, m.cursor.runeKey), "link copied")
	case msg.String() == "c" && m.cursor.kind == ui.RowLookout:
		return m.copyToClipboard(m.cursor.lookoutURL, "link copied")
	case msg.String() == "c" && m.cursor.kind == ui.RowTrail:
		return m.copyToClipboard(prURL(m.cursor.prRepo, m.cursor.prCode), "link copied")
	case msg.String() == "r" && isTrailResyncRow(m.cursor.kind):
		// Resync this quest's trails from any room (refetch CI/comments/approvals,
		// re-scan for tracks/runes). RowLookout keeps "r" for rename (handled above).
		if q := m.findQuest(m.cursor.questID); q != nil {
			return m.resyncTrails(q)
		}
		return nil
	}

	// Everything else — printable characters, arrows, Home/End, Ctrl+A/E/K/U/W
	// — is normal text editing, forwarded to the live row editor. Shift+arrow/
	// Home/End extend a text selection instead (see applySelectionKey) and
	// aren't forwarded any further.
	if m.editor != nil {
		if handled, cmd := m.applySelectionKey(m.editor, msg); handled {
			return cmd
		}
		var cmd tea.Cmd
		*m.editor, cmd = m.editor.Update(msg)
		return cmd
	}
	return nil
}

// moveCursor steps to the previous/next selectable row, leaving a fading trail
// ghost — the keyboard "hop". (The mouse wheel scrolls the viewport instead and
// never touches the cursor — see handleWheel.)
func (m *Model) moveCursor(delta int) {
	fromNote := ""
	if m.cursor.kind == ui.RowBodyLine {
		fromNote = m.cursor.projectID
	}
	m.commitEdit()
	// Arrowing off an unnamed Tavern draft discards it (and relocates the
	// cursor to a safe neighbor) — that hop is the whole move.
	if m.discardEmptyPaneDraft() {
		return
	}
	defer func() {
		// Leaving a campaign's notes for a non-note row drops any dangling
		// trailing blank line the session left behind.
		if fromNote != "" && m.cursor.kind != ui.RowBodyLine {
			m.pruneTrailingEmptyNotes(fromNote)
		}
	}()
	rows := m.visibleRows()
	if len(rows) == 0 {
		return
	}
	idx := findRowIndex(rows, m.cursor)
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
	// The trail ghost is dropped where the cursor was (cursorScreen{X,Y} still
	// holds the last-rendered cell), then the cursor moves instantly.
	m.spawnCursorTrail(m.cursorScreenX, m.cursorScreenY)
	m.setCursor(rows[next])
}

// moveRow (Shift+Up/Down) reorders whatever's under the cursor — a quest
// among its siblings, or a campaign among the others.
func (m *Model) moveRow(delta int) {
	if m.wilds {
		// The Wilds has its own manual order, independent of the Tavern.
		m.moveWildsQuest(delta)
		return
	}
	switch m.cursor.kind {
	case ui.RowQuest:
		m.moveQuest(delta)
	case ui.RowProject:
		m.moveProject(delta)
	}
}

// moveQuest swaps the current quest with the nearest quest in the given
// direction that shares its sort tier (see ui.SortBucket) — so a reorder
// can rearrange side quests among themselves, mains among themselves, and
// priority among themselves, but never lift a quest across a tier boundary
// (a side above a main, a main above priority). With no top-toggles on,
// every quest is one tier, so anything can move past anything. Hitting a
// different tier or a non-quest row first means there's nothing to swap
// with in that direction, so it's a no-op.
func (m *Model) moveQuest(delta int) {
	rows := m.currentRowScope()
	idx := findRowIndex(rows, m.cursor)
	if idx < 0 {
		return
	}
	me := m.findQuest(m.cursor.questID)
	if me == nil {
		return
	}

	j := idx
	for {
		j += delta
		if j < 0 || j >= len(rows) {
			return
		}
		r := rows[j]
		if !r.Selectable() {
			continue
		}
		if r.Kind != ui.RowQuest {
			return
		}
		other := m.findQuest(r.QuestID)
		if other == nil || ui.SortBucket(*other) != ui.SortBucket(*me) {
			return
		}
		m.swapQuests(me.ID, other.ID)
		return
	}
}

// reslotToBucketTop moves the quest to the top of its sort tier in the store
// (right before the first same-campaign, same-tier sibling) — call it after
// a toggle changes which tier a quest belongs to, so it floats to the top
// of the group it just joined instead of keeping a now-arbitrary position.
// If it's alone in its tier there's nothing to reorder against, so it's a
// no-op (the tier's sort key already places it correctly).
func (m *Model) reslotToBucketTop(id string) {
	qs := m.store.Quests
	from := -1
	for i := range qs {
		if qs[i].ID == id {
			from = i
			break
		}
	}
	if from < 0 {
		return
	}
	q := qs[from]
	tier := ui.SortBucket(q)

	target := -1
	for i := range qs {
		if i == from || qs[i].Vaulted || qs[i].ProjectID != q.ProjectID {
			continue
		}
		if ui.SortBucket(qs[i]) == tier {
			target = i
			break
		}
	}
	if target < 0 {
		return
	}

	qs = append(qs[:from], qs[from+1:]...)
	if from < target {
		target--
	}
	qs = append(qs, model.Quest{})
	copy(qs[target+1:], qs[target:])
	qs[target] = q
	m.store.Quests = qs
}

// reslotIfTierChanged floats id to the top of its tier only when `before`
// (its tier prior to a toggle) differs from its tier now — so toggling a
// property that doesn't affect ordering (any toggle while its top-config is
// off) leaves the quest exactly where it was.
func (m *Model) reslotIfTierChanged(id string, before int) {
	if q := m.findQuest(id); q != nil && ui.SortBucket(*q) != before {
		m.reslotToBucketTop(id)
	}
}

func (m *Model) swapQuests(idA, idB string) {
	ia, ib := -1, -1
	for i, q := range m.store.Quests {
		if q.ID == idA {
			ia = i
		}
		if q.ID == idB {
			ib = i
		}
	}
	if ia < 0 || ib < 0 {
		return
	}
	m.store.Quests[ia], m.store.Quests[ib] = m.store.Quests[ib], m.store.Quests[ia]
	m.save()
}

// moveProject swaps the current campaign with the nearest campaign in the
// given direction that shares its archived state (campaigns and archived
// campaigns render in separate, non-interleaved lists).
// moveBanner swaps the hall-selected banner with its neighbor (delta -1/+1) in
// the sidebar order, keeping the selection on it. A no-op on the synthetic
// "Unassigned" area (not a real banner) or at either end.
func (m *Model) moveBanner(delta int) {
	idx := -1
	for i := range m.store.Banners {
		if m.store.Banners[i].ID == m.hallCursor.bannerID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	j := idx + delta
	if j < 0 || j >= len(m.store.Banners) {
		return
	}
	m.store.Banners[idx], m.store.Banners[j] = m.store.Banners[j], m.store.Banners[idx]
	m.save()
	m.cursorMoved = true // keep the hall scroll following the moved banner
}

func (m *Model) moveProject(delta int) {
	me := m.findProject(m.cursor.projectID)
	if me == nil {
		return
	}
	idx := -1
	for i, p := range m.store.Projects {
		if p.ID == me.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	j := idx
	for {
		j += delta
		if j < 0 || j >= len(m.store.Projects) {
			return
		}
		if m.store.Projects[j].Archived == me.Archived {
			m.store.Projects[idx], m.store.Projects[j] = m.store.Projects[j], m.store.Projects[idx]
			m.save()
			return
		}
	}
}

// toggleReveal expands/collapses the campaign or section under the cursor —
// used by clicking a row's disclosure caret, and by Enter on a campaign or
// section header (see handleEnter).
func (m *Model) toggleReveal() {
	switch m.cursor.kind {
	case ui.RowProject:
		m.collapsedProjects[m.cursor.projectID] = !m.collapsedProjects[m.cursor.projectID]
	case ui.RowBanner:
		// Banners fold their campaigns; reuse collapsedProjects keyed by banner ID.
		m.collapsedProjects[m.cursor.bannerID] = !m.collapsedProjects[m.cursor.bannerID]
	case ui.RowSection:
		m.collapsedSections[m.cursor.section] = !m.collapsedSections[m.cursor.section]
		m.saveLayoutConfig()
	case ui.RowVaultHeader:
		// A focused page's Vaulted group — expanded state under a synthetic key.
		k := ui.VaultOpenKey(m.cursor.section)
		m.collapsedProjects[k] = !m.collapsedProjects[k]
	}
	m.invalidateRender()
}

// handleReveal is Tab / a section-name click: opens a focused, full-screen
// detail view for a campaign, a quest, or a section. Every section
// (Questboard / Runes / Campaigns / Vault) has one; the "Campaigns" label opens
// the campaigns section view.
func (m *Model) handleReveal() tea.Cmd {
	// Tab is navigation in the Tavern: a campaign row (or a saga chapter link)
	// drills into that campaign's view.
	if m.inTavern() && !m.hallFocus && (m.cursor.kind == ui.RowProject || m.cursor.kind == ui.RowSagaLink) {
		m.drillIntoCampaign(m.cursor.projectID)
		return nil
	}
	switch m.cursor.kind {
	case ui.RowProject:
		// Tab on a campaign header jumps to that campaign's Tavern pane — the
		// campaign's home, where its notes and quests live.
		m.commitEdit()
		if p := m.findProject(m.cursor.projectID); p != nil {
			m.drillIntoCampaign(p.ID)
		}
	case ui.RowQuest, ui.RowWildsObjective:
		// An objective opens its parent quest — the mouse/Tab equivalent of
		// stepping into the quest that owns it.
		m.commitEdit()
		if q := m.findQuest(m.cursor.questID); q != nil {
			m.openQuestDetail(q)
		}
	case ui.RowRune, ui.RowRuneQuest, ui.RowLookout, ui.RowLookoutQuest,
		ui.RowTrack, ui.RowTrail, ui.RowTrailQuest:
		// A sigil in the Trails/Lookouts/Runes rooms (a PR, dashboard, flag, or
		// event) belongs to a quest — Tab opens that quest's page so you can see
		// what references it. (Enter still opens the sigil's own link/collapse.)
		m.commitEdit()
		if q := m.findQuest(m.cursor.questID); q != nil {
			m.openQuestDetail(q)
		}
	case ui.RowSection, ui.RowLabel:
		m.commitEdit()
		section := m.cursor.section
		if m.cursor.kind == ui.RowLabel { // the "Campaigns" banner
			section = "campaigns"
		}
		m.pushModal(sectionDetailModal(section))
		if r, ok := nearestSelectableRow(m.sectionRows(section), 0); ok {
			m.setCursor(r)
		} else {
			m.editor = nil
		}
	}
	return nil
}

// handleEnter inserts a new sibling row right after the cursor, in edit
// mode. On the "+ New Project"/"+ New Quest" rows it creates a new
// campaign/quest instead. On a campaign header, a section header, or the
// "Campaigns" label it toggles collapse — of that one campaign, that
// section, or every campaign at once, respectively.
func (m *Model) handleEnter() tea.Cmd {
	// In Camp (not in a modal), Enter on a taken quest ventures it into the
	// Wilds — the focus ritual. Camp is an agenda, not an outliner, so Enter
	// commits to one quest rather than adding a sibling.
	if m.modal == nil && m.wilds && !m.venturing() && m.cursor.kind == ui.RowQuest {
		return m.venture(m.cursor.questID)
	}
	// The Wilds is distraction-free: Enter makes no structural edits there.
	if m.modal == nil && m.venturing() {
		return nil
	}
	// In the Tavern content pane, Enter ADDS a new sibling below the cursor
	// (Tab is navigation now). Contextual: a quest next to a quest, a campaign
	// next to a campaign, matching the group's status. A saga chapter link is
	// navigation, not a row to add beside — Enter drills into it.
	if m.modal == nil && m.inTavern() && !m.hallFocus {
		if m.cursor.kind == ui.RowSagaLink {
			m.drillIntoCampaign(m.cursor.projectID)
			return nil
		}
		return m.contextualCreate()
	}

	rows := m.currentRowScope()
	idx := findRowIndex(rows, m.cursor)
	if idx < 0 {
		return nil
	}
	row := rows[idx]
	m.commitEdit()

	switch row.Kind {
	case ui.RowNewProject:
		p := model.Project{ID: store.NewID(), Name: "", BannerID: row.BannerID}
		m.store.Projects = append(m.store.Projects, p)
		m.save()
		m.setCursor(ui.Row{Kind: ui.RowProject, ProjectID: p.ID})

	case ui.RowNewBanner:
		b := model.Banner{ID: store.NewID(), Name: ""}
		m.store.Banners = append(m.store.Banners, b)
		m.save()
		m.setCursor(ui.Row{Kind: ui.RowBanner, BannerID: b.ID})

	case ui.RowNewQuest:
		if row.ProjectID != "" {
			p := m.findProject(row.ProjectID)
			if p == nil || p.Archived {
				return nil
			}
			m.collapsedProjects[row.ProjectID] = false
		}
		q := m.newQuestUnder(row.ProjectID, model.StatusOpen)
		if row.ProjectID == "" && row.BannerID != "" {
			// A loose quest lives directly under a banner (ongoing area work).
			if nq := m.findQuest(q.ID); nq != nil {
				nq.BannerID = row.BannerID
				m.save()
			}
			m.collapsedProjects[row.BannerID] = false // keep the banner open
		}
		m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: q.ProjectID, QuestID: q.ID})

	case ui.RowProject, ui.RowSection, ui.RowVaultHeader, ui.RowBanner:
		m.toggleReveal()

	case ui.RowTrailQuest, ui.RowRuneQuest, ui.RowLookoutQuest:
		// Room quest headers aren't collapsible — Enter resyncs that quest's trails
		// (refetch PR status, re-scan for tracks/runes). A no-op if it has no PRs.
		if q := m.findQuest(m.cursor.questID); q != nil {
			return m.resyncTrails(q)
		}

	case ui.RowRune:
		return m.openConnection(connection{kind: linkRune, code: row.RuneKey, url: ldFlagURL(m.ldProject, m.ldEnv, row.RuneKey)})

	case ui.RowLookout:
		return m.openLookoutScry(row.LookoutURL)

	case ui.RowTrail:
		return openURL(prURL(row.Repo, row.Code))

	case ui.RowLabel:
		m.toggleAllCampaigns()

	case ui.RowQuest:
		q := m.findQuest(row.QuestID)
		if q == nil || m.isVaulted(q) {
			return nil
		}
		// Inherit the context of the row Enter was pressed on, but never
		// inherit Done — a brand new quest is never already finished.
		status := q.Status
		if status == model.StatusDone {
			status = model.StatusOpen
		}
		newQ := m.newQuestUnder(q.ProjectID, status)
		m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: newQ.ProjectID, QuestID: newQ.ID})
	}
	return nil
}

// isVaulted reports whether a quest currently lives in the Vault — either
// directly (parked via Ctrl+V) or because its campaign is archived.
func (m *Model) isVaulted(q *model.Quest) bool {
	if q.Vaulted {
		return true
	}
	if q.ProjectID != "" {
		if p := m.findProject(q.ProjectID); p != nil && p.Archived {
			return true
		}
	}
	return false
}

// contextualCreate adds a new sibling below the cursor in a Tavern pane — a
// quest next to a quest (same campaign/loose banner, inheriting active/open), a
// campaign next to a campaign, a quest on a campaign's header, a campaign on a
// banner's header. Both Enter and Ctrl+N use it; the new row opens for naming.
func (m *Model) contextualCreate() tea.Cmd {
	m.commitEdit()
	switch m.cursor.kind {
	case ui.RowQuest, ui.RowNewQuest:
		return m.createSiblingQuest()
	case ui.RowNewProject:
		m.createCampaignInPane(m.cursor.bannerID)
	case ui.RowProject:
		if m.hallCursor.kind == ui.RowProject && m.cursor.projectID == m.hallCursor.projectID {
			// The campaign-view header → add a (new, inactive) quest to it.
			nq := m.newQuestUnder(m.cursor.projectID, model.StatusOpen)
			m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: m.cursor.projectID, QuestID: nq.ID, Bare: true})
		} else if p := m.findProject(m.cursor.projectID); p != nil {
			m.createCampaignInPane(p.BannerID)
		}
	case ui.RowBanner:
		m.createCampaignInPane(m.cursor.bannerID)
	case ui.RowSection:
		if m.cursor.section == "inbox" {
			nq := m.newQuestUnder("", model.StatusOpen)
			m.setCursor(ui.Row{Kind: ui.RowQuest, QuestID: nq.ID, Bare: true})
		}
	case ui.RowDayHeader: // the "Later" divider
		if m.hallCursor.kind == ui.RowProject {
			nq := m.newQuestUnder(m.hallCursor.projectID, model.StatusOpen)
			m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: m.hallCursor.projectID, QuestID: nq.ID, Bare: true})
		} else if m.hallCursor.kind == ui.RowBanner {
			m.createCampaignInPane(m.hallCursor.bannerID)
		}
	case ui.RowVaultHeader:
		m.toggleReveal()
	}
	return nil
}

// createSiblingQuest adds a quest next to the cursor quest (or on a "+ New Quest"
// row) — same campaign or loose banner, inheriting its active/open status.
func (m *Model) createSiblingQuest() tea.Cmd {
	projectID, bannerID, status := "", "", model.StatusOpen
	switch m.cursor.kind {
	case ui.RowQuest:
		q := m.findQuest(m.cursor.questID)
		if q == nil {
			return nil
		}
		projectID, bannerID, status = q.ProjectID, q.BannerID, q.Status
		if status == model.StatusDone {
			status = model.StatusOpen
		}
	case ui.RowNewQuest:
		// "+ add quest" adds an inactive (open) quest — you take it up later.
		projectID, bannerID = m.cursor.projectID, m.cursor.bannerID
	}
	nq := m.newQuestUnder(projectID, status)
	if projectID == "" && bannerID != "" {
		if x := m.findQuest(nq.ID); x != nil {
			x.BannerID = bannerID
			m.save()
		}
	}
	m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: projectID, QuestID: nq.ID, Bare: true})
	return nil
}

// createCampaignInPane adds a campaign under bannerID and drops the pane cursor
// on it to name it. Under the pinned Errands area, which holds no campaigns, it
// adds a loose quest instead.
func (m *Model) createCampaignInPane(bannerID string) {
	if bannerID == errandsBanner {
		m.createLooseQuestInPane(bannerID)
		return
	}
	np := model.Project{ID: store.NewID(), Name: "", BannerID: bannerID}
	m.store.Projects = append(m.store.Projects, np)
	m.save()
	m.setCursor(ui.Row{Kind: ui.RowProject, ProjectID: np.ID})
}

// createLooseQuestInPane adds an open loose quest directly under a banner (no
// campaign) and drops the pane cursor on it to name it.
func (m *Model) createLooseQuestInPane(bannerID string) {
	nq := m.newQuestUnder("", model.StatusOpen)
	if x := m.findQuest(nq.ID); x != nil {
		x.BannerID = bannerID
		m.save()
	}
	m.setCursor(ui.Row{Kind: ui.RowQuest, QuestID: nq.ID, Bare: true})
}

// hallCreate (hall Enter) adds a campaign in the current area and dives into it
// to name it; on a "+ New" affordance it creates as before.
func (m *Model) hallCreate() tea.Cmd {
	m.commitEdit()
	switch m.hallCursor.kind {
	case ui.RowNewBanner, ui.RowNewProject:
		return m.activateHallEntry()
	case ui.RowBanner:
		return m.createCampaignFromHall(m.hallCursor.bannerID)
	case ui.RowProject:
		if p := m.findProject(m.hallCursor.projectID); p != nil {
			return m.createCampaignFromHall(p.BannerID)
		}
	}
	return nil
}

func (m *Model) createCampaignFromHall(bannerID string) tea.Cmd {
	if bannerID == errandsBanner {
		// Errands hold quests, not campaigns — dive into its pane and add a quest.
		m.diveIntoPane()
		m.createLooseQuestInPane(bannerID)
		return nil
	}
	np := model.Project{ID: store.NewID(), Name: "", BannerID: bannerID}
	m.store.Projects = append(m.store.Projects, np)
	m.save()
	m.hallCursor = cursorTarget{kind: ui.RowProject, projectID: np.ID}
	m.diveIntoPane() // land on the new campaign's header to name it
	return nil
}

// newBannerFromHall (Ctrl+N in the hall) creates a Banner and dives in to name it.
func (m *Model) newBannerFromHall() tea.Cmd {
	m.commitEdit()
	m.hallCursor = cursorTarget{kind: ui.RowNewBanner}
	return m.activateHallEntry()
}

func (m *Model) newQuestUnder(projectID string, status model.QuestStatus) model.Quest {
	now := time.Now()
	q := model.Quest{
		ID:        store.NewID(),
		Title:     "",
		Type:      model.QuestTypeSide,
		Status:    status,
		ProjectID: projectID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.store.Quests = append(m.store.Quests, q)
	m.save()
	return q
}

// handleBackspace deletes an empty row outright (cursor at column 0 of an
// empty title) — the usual outliner "merge/remove empty line" behavior. If
// the quest has details (a non-empty body), it's not deleted silently — the
// confirm dialog opens instead (see openDeleteModal), since the empty title
// could otherwise hide real data being lost. Deleting a project this way
// requires it to already be empty of quests; otherwise Ctrl+X (with the
// confirm dialog) is required.
func (m *Model) handleBackspace(msg tea.KeyPressMsg) tea.Cmd {
	if m.editor == nil {
		return nil
	}
	if start, end, ok := m.selectionBounds(m.editor); ok {
		runes := []rune(m.editor.Value())
		m.editor.SetValue(string(runes[:start]) + string(runes[end:]))
		m.editor.SetCursor(start)
		m.clearSelection()
		return nil
	}
	if m.editor.Value() == "" && m.editor.Position() == 0 {
		switch m.cursor.kind {
		case ui.RowQuest:
			id := m.cursor.questID
			if q := m.findQuest(id); q != nil && questHasDetails(q) {
				m.openDeleteModal(m.cursor) // has notes — confirm rather than silently drop
				return nil
			}
			m.removeCurrentRow(func() { m.deleteQuestByID(id) })
		case ui.RowProject:
			id := m.cursor.projectID
			if m.projectQuestCount(id) == 0 {
				m.removeCurrentRow(func() { m.deleteProjectByID(id) })
			}
		}
		return nil
	}

	var cmd tea.Cmd
	*m.editor, cmd = m.editor.Update(msg)
	return cmd
}

func questHasDetails(q *model.Quest) bool {
	for _, l := range q.Body {
		if strings.TrimSpace(l.Text) != "" {
			return true
		}
	}
	return false
}

// toggleDone and toggleActive are no-ops on a Questboard quest (listing-only,
// nothing to mark done/active until it's picked up via Ctrl+P) and on a
// vaulted quest — the Vault is read-only for status changes; a parked quest
// keeps whatever done/active state it had when it was sent there. Trying
// either on a vaulted quest shows a brief inline warning instead of just
// silently doing nothing.
// toggleCampaignDone marks a campaign done (done-styled) or reopens it. It stays
// in the hall either way — sending it to the Vault is a separate, manual Ctrl+V
// (toggleVault), same as a quest. Archived campaigns are left alone.
func (m *Model) toggleCampaignDone(projectID string) tea.Cmd {
	p := m.findProject(projectID)
	if p == nil || p.Archived {
		return nil
	}
	if p.IsCompleted() {
		p.CompletedAt = nil
		m.save()
		return nil
	}
	now := time.Now()
	p.CompletedAt = &now
	m.save()
	return m.playSound(sndQuestDone)
}

func (m *Model) toggleDone() tea.Cmd {
	if m.cursor.kind == ui.RowWildsObjective {
		return m.markWildsObjectiveDone()
	}
	// Ctrl+D on the quest you've ventured with finishes it — mark done, then
	// return to Camp with the flourish (see completeVenture).
	if m.modal == nil && m.venturing() && m.cursor.kind == ui.RowQuest && m.cursor.questID == m.venturedID {
		return m.completeVenture()
	}
	if m.cursor.kind == ui.RowProject {
		return m.toggleCampaignDone(m.cursor.projectID) // mark done; it stays in the hall
	}
	if m.cursor.kind != ui.RowQuest {
		return nil
	}
	q := m.findQuest(m.cursor.questID)
	if q == nil || q.InQuestboard() {
		return nil
	}
	if m.isVaulted(q) {
		return m.showWarning(m.cursor, "vault is read-only")
	}
	before := ui.SortBucket(*q)
	becameDone, recycled := false, false
	if q.Status == model.StatusDone {
		q.Status = model.StatusOpen
		q.CompletedAt = nil
	} else if recycleRite(q) {
		// A rite doesn't finish — it rolls to its next muster and leaves Camp.
		recycled = true
	} else {
		q.Status = model.StatusDone
		now := time.Now()
		q.CompletedAt = &now
		becameDone = true
	}
	q.UpdatedAt = time.Now()
	m.reslotIfTierChanged(q.ID, before)
	m.save()
	if becameDone || recycled {
		// A quest gets the ring "seal stamp"; objectives get the scatter burst.
		m.spawnSparkleRing(m.cursorScreenX+questGlyphCol, m.cursorScreenY, 12)
		cmds := []tea.Cmd{m.playSound(sndQuestDone), m.maybeStartOverlayTick()}
		if recycled {
			when, _ := ui.MusterWhen(*q.Muster, time.Now())
			cmds = append(cmds, m.showClipboardToastText("rite done · next "+when))
		}
		return tea.Batch(cmds...)
	}
	return nil
}

// markWildsObjectiveDone checks off the objective under the cursor in the
// Wilds and lands the cursor on whatever row takes its place (the next
// objective, or the parent quest once the last one is cleared) — the objective
// row itself drops out of the list, since the Wilds only lists pending ones.
func (m *Model) markWildsObjectiveDone() tea.Cmd {
	q := m.findQuest(m.cursor.questID)
	if q == nil {
		return nil
	}
	idx := findRowIndex(m.visibleRows(), m.cursor)
	objIndent := 0
	for i := range q.Body {
		if q.Body[i].ID == m.cursor.bodyLineID {
			q.Body[i].Done = true
			objIndent = q.Body[i].Indent
			break
		}
	}
	q.UpdatedAt = time.Now()
	m.save()
	// Clearing the last pending objective out in the Wilds finishes the quest —
	// the completion moment of the focus ritual (its own flourish subsumes the
	// objective ding).
	if m.modal == nil && m.venturing() && m.venturedID == q.ID {
		if _, more := q.NextObjective(); !more {
			return m.completeVenture()
		}
	}
	if row, ok := nearestSelectableRow(m.visibleRows(), idx); ok {
		m.setCursor(row)
	}
	m.spawnSparkleBurst(m.cursorScreenX+wildsObjCol+2*objIndent, m.cursorScreenY, 10)
	return tea.Batch(m.playSound(sndObjectiveDone), m.maybeStartOverlayTick())
}

// toggleActive follows the same "set this status, or back to Open if it's
// already set" pattern as toggleDone — every status is a single field, so
// setting one always clears whatever was there before.
func (m *Model) toggleActive() tea.Cmd {
	if m.cursor.kind != ui.RowQuest {
		return nil
	}
	q := m.findQuest(m.cursor.questID)
	if q == nil {
		return nil
	}
	if q.InQuestboard() {
		// A Questboard quest has no campaign to be active under yet — Ctrl+A
		// triages it: pick a campaign and take it up (file as active) in one step.
		m.commitEdit()
		mod := projectPickerModal(m.store, q.ID, "", true)
		mod.SourceRowIdx = findRowIndex(m.currentRowScope(), m.cursor)
		m.pushModal(mod)
		return nil
	}
	if m.isVaulted(q) {
		return m.showWarning(m.cursor, "vault is read-only")
	}
	becameActive := false
	if q.Status == model.StatusActive {
		q.Status = model.StatusOpen
	} else {
		q.Status = model.StatusActive
		becameActive = true
	}
	q.UpdatedAt = time.Now()
	m.save()
	if becameActive {
		return m.playSound(sndQuestActive)
	}
	return nil
}

// toggleVault sends whatever's under the cursor to (or back out of) the
// Vault — a quest is parked via its Vaulted flag, a campaign via its
// Archived flag. One shortcut, since both are simply "not currently active".
// Vaulting a quest doesn't touch its Status, so it keeps whatever done/
// active state it had — Vaulted is a separate axis from Status.
func (m *Model) toggleVault() {
	switch m.cursor.kind {
	case ui.RowQuest:
		q := m.findQuest(m.cursor.questID)
		if q == nil {
			return
		}
		q.Vaulted = !q.Vaulted
		now := time.Now()
		q.UpdatedAt = now
		if q.Vaulted {
			// Anything parked in the Vault is considered finished with — mark it
			// done and stamp when it went in (drives the Vault's day timeline).
			q.Status = model.StatusDone
			q.CompletedAt = &now
			q.VaultedAt = &now
		} else {
			q.VaultedAt = nil
		}
		m.save()
	case ui.RowProject, ui.RowVaultCampaign:
		p := m.findProject(m.cursor.projectID)
		if p == nil {
			return
		}
		p.Archived = !p.Archived
		if p.Archived {
			now := time.Now()
			p.ArchivedAt = &now // places it in the Vault's day timeline
		} else {
			p.ArchivedAt = nil
		}
		m.save()
	}
}

func (m *Model) toggleType() {
	if m.cursor.kind != ui.RowQuest {
		return
	}
	q := m.findQuest(m.cursor.questID)
	if q == nil {
		return
	}
	before := ui.SortBucket(*q)
	if q.Type == model.QuestTypeMain {
		q.Type = model.QuestTypeSide
	} else {
		q.Type = model.QuestTypeMain
	}
	q.UpdatedAt = time.Now()
	m.reslotIfTierChanged(q.ID, before)
	m.save()
}

// nextPriority is the cycle the priority key steps through:
// none → medium → high → low → none.
func nextPriority(p model.Priority) model.Priority {
	switch p {
	case model.PriorityNone:
		return model.PriorityMedium
	case model.PriorityMedium:
		return model.PriorityHigh
	case model.PriorityHigh:
		return model.PriorityLow
	default:
		return model.PriorityNone
	}
}

// toggleImportant flags/unflags a quest as priority work (an orthogonal
// axis from type/status, so it applies on the Questboard too). Blocked in
// the read-only Vault, matching the other toggles.
func (m *Model) cyclePriority() tea.Cmd {
	switch m.cursor.kind {
	case ui.RowProject:
		m.cycleProjectPriority(m.cursor.projectID)
		return nil
	case ui.RowQuest:
		q := m.findQuest(m.cursor.questID)
		if q == nil {
			return nil
		}
		if m.isVaulted(q) {
			return m.showWarning(m.cursor, "vault is read-only")
		}
		before := ui.SortBucket(*q)
		q.Priority = nextPriority(q.Priority)
		q.UpdatedAt = time.Now()
		m.reslotIfTierChanged(q.ID, before)
		m.save()
	}
	return nil
}

// onQuestInDetail runs a list quest-action (toggleActive/Done/Type/Vault,
// cyclePriority, openProjectPicker — all of which target m.cursor) against the
// quest open in the detail page, then restores the real cursor. This lets every
// list action work from inside a quest, not only from a list.
func (m *Model) onQuestInDetail(q *model.Quest, action func() tea.Cmd) tea.Cmd {
	saved := m.cursor
	m.cursor = cursorTarget{kind: ui.RowQuest, questID: q.ID, projectID: q.ProjectID}
	cmd := action()
	m.cursor = saved
	return cmd
}

// bodyCursorOnObjective reports whether the body line being edited is an
// objective ("- …"), so Ctrl+D checks it off rather than toggling the whole quest
// done. Reads the live editor value (the current line may be uncommitted).
func (m *Model) bodyCursorOnObjective() bool {
	kind, _ := model.ClassifyBodyLine(m.bodyEditor.Value())
	return kind == model.BodyObjective
}

// cycleProjectPriority steps a campaign's priority marker (none → med → high →
// low → none). It's a visual cue only — campaign order is unchanged.
func (m *Model) cycleProjectPriority(projectID string) {
	p := m.findProject(projectID)
	if p == nil {
		return
	}
	p.Priority = nextPriority(p.Priority)
	m.save()
}

func (m *Model) openProjectPicker() {
	m.commitEdit()
	srcIdx := findRowIndex(m.currentRowScope(), m.cursor)
	switch m.cursor.kind {
	case ui.RowQuest:
		q := m.findQuest(m.cursor.questID)
		if q == nil {
			return
		}
		// Pre-select the quest's current home — its campaign, or its area (a loose
		// quest under a banner) via the bannerMoveTarget sentinel.
		current := q.ProjectID
		if current == "" && q.BannerID != "" {
			current = bannerMoveTarget + q.BannerID
		}
		mod := projectPickerModal(m.store, q.ID, current, false)
		mod.SourceRowIdx = srcIdx
		m.pushModal(mod)
	case ui.RowProject:
		// On a campaign, "move to…" picks its Banner (Area) instead.
		p := m.findProject(m.cursor.projectID)
		if p == nil {
			return
		}
		mod := bannerPickerModal(m.store, p.ID, p.BannerID)
		mod.SourceRowIdx = srcIdx
		m.pushModal(mod)
	}
}

func (m *Model) deleteQuestByID(id string) {
	out := m.store.Quests[:0]
	for _, q := range m.store.Quests {
		if q.ID != id {
			out = append(out, q)
		}
	}
	m.store.Quests = out
	m.save()
}

func (m *Model) deleteProjectByID(id string) {
	outP := m.store.Projects[:0]
	for _, p := range m.store.Projects {
		if p.ID != id {
			outP = append(outP, p)
		}
	}
	m.store.Projects = outP

	outQ := m.store.Quests[:0]
	for _, q := range m.store.Quests {
		if q.ProjectID != id {
			outQ = append(outQ, q)
		}
	}
	m.store.Quests = outQ
	m.save()
}

// deleteBannerByID removes a banner, ungrouping its campaigns and loose quests
// (their work survives — campaigns fall back to Unassigned, loose quests to the
// Questboard) rather than deleting them. Returns how many items were ungrouped.
func (m *Model) deleteBannerByID(id string) int {
	ungrouped := 0
	for i := range m.store.Projects {
		if m.store.Projects[i].BannerID == id {
			m.store.Projects[i].BannerID = ""
			ungrouped++
		}
	}
	for i := range m.store.Quests {
		if m.store.Quests[i].BannerID == id {
			m.store.Quests[i].BannerID = ""
			ungrouped++
		}
	}
	out := m.store.Banners[:0]
	for _, b := range m.store.Banners {
		if b.ID != id {
			out = append(out, b)
		}
	}
	m.store.Banners = out
	m.save()
	return ungrouped
}

// titleOffset is the fixed number of display columns before a row's
// editable title text starts, matching RenderRow's own layout exactly —
// used to convert a click/drag's screen column into a rune index for text
// selection (see beginTextSelection/dragTextSelection).
func titleOffset(row ui.Row, nestOffset int) int {
	if row.Kind == ui.RowProject {
		return 4 + nestOffset
	}
	return 6 + nestOffset // cursor(2) + priority(2) + glyph(1) + space(1)
}

// handleClick handles a left mouse press — everything a v1 tea.MouseMsg with
// Action==Press used to reach. Right/middle presses are ignored (matching v1,
// which only ever checked Button==Left here).
func (m *Model) handleClick(msg tea.MouseClickMsg) tea.Cmd {
	if m.inTavern() {
		return m.handleTavernClick(msg) // the borderless hall + pane has its own routing
	}
	mouse := msg.Mouse()
	m.hoverSection = ""

	if mouse.Button != tea.MouseLeft {
		return nil
	}

	rows := m.visibleRows()
	if len(rows) == 0 {
		return nil
	}

	// The header row holds the TAVERN/WILDS toggle (centered) and the right-
	// aligned "F1 help" button.
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

	relY := mouse.Y - m.rowsScreenTop
	if relY < 0 {
		return nil // clicked the logo/blank area above the rows
	}
	idx := m.scrollOffset + relY
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	return m.clickRowAt(rows, idx, mouse, m.leftMargin)
}

// handleWheel handles a scroll-wheel event — the v1 equivalent lived inside
// handleMouse guarded by Action==Press && Button==Wheel*.
func (m *Model) handleWheel(msg tea.MouseWheelMsg) tea.Cmd {
	if m.inTavern() {
		return m.handleTavernWheel(msg)
	}
	mouse := msg.Mouse()
	delta := 1
	if mouse.Button == tea.MouseWheelUp {
		delta = -1
	}
	// The wheel scrolls the VIEWPORT only — it never moves the cursor (which
	// stays put, possibly scrolling out of view, like a normal editor). Every
	// scrollable surface follows this rule; each render re-centers on the cursor
	// only when the cursor itself moved (cursorMoved), so a wheel scroll sticks.
	// Clamp against the last render's max so scrolling past an end is a true
	// no-op — no state churn, no re-render (which would otherwise flicker).
	next := clampInt(m.scrollOffset+delta, 0, m.scrollMax)
	if next == m.scrollOffset {
		return nil
	}
	m.scrollOffset = next
	m.invalidateRender()
	return nil
}

// handleMotion handles mouse movement — v1's Action==Motion case. A resize
// drag in progress owns motion until release; otherwise a held left button
// (m.leftDown, set in Update's MouseClickMsg case) drags a text selection,
// and with no button held it's just hover tracking.
func (m *Model) handleMotion(msg tea.MouseMotionMsg) tea.Cmd {
	mouse := msg.Mouse()
	if m.resizeDrag.active {
		m.updateResizeDrag(mouse.X, mouse.Y)
		return nil
	}
	rows := m.visibleRows()
	if len(rows) == 0 {
		return nil
	}
	if m.leftDown {
		return m.dragTextSelection(mouse, rows)
	}
	m.updateHover(mouse, rows)
	return nil
}

// commonRowClick performs the click actions that are identical on every list
// surface — opening a rune, collapsing a rune-quest group, or adding via a
// "+ New …" affordance. It returns
// (cmd, true) when it handled the row kind; (nil, false) leaves the
// surface-specific kinds (Section chevron, Quest checkbox — whose click zones
// depend on that surface's layout) to the caller. One definition of what a
// click does to these rows, so the Tavern rail and campaigns/Wilds column
// can't drift apart (the kind of split that hid the rune-click bug).
func (m *Model) commonRowClick(row ui.Row) (tea.Cmd, bool) {
	switch row.Kind {
	case ui.RowRune:
		// A click copies the flag link (double-click opens); Enter still opens
		// via applyRowAction — matching links everywhere else.
		return m.clickLink(ldFlagURL(m.ldProject, m.ldEnv, row.RuneKey)), true
	case ui.RowVaultHeader:
		m.toggleReveal()
		return nil, true
	case ui.RowTrailQuest, ui.RowRuneQuest, ui.RowLookoutQuest:
		// Room quest headers aren't collapsible — a click resyncs that quest.
		if q := m.findQuest(row.QuestID); q != nil {
			return m.resyncTrails(q), true
		}
		return nil, true
	case ui.RowLookout:
		return m.clickLink(row.LookoutURL), true
	case ui.RowTrail:
		return m.clickLink(prURL(row.Repo, row.Code)), true
	case ui.RowTrack:
		return m.copyTrack(row.QuestID, row.TrackEvent), true // click copies the event + marks
	case ui.RowNewProject, ui.RowNewQuest:
		return m.handleEnter(), true
	}
	return nil, false
}

// clickRowAt handles a left-press on the row at idx of rows: opening a meta
// line's code, moving the cursor, firing a clicked hint, or the per-kind
// default (toggle collapse/done, open a rune, add). colX is the column's
// content left edge — relX is measured from it, matching how hint/code spans
// were recorded.
// rowDoubleClickWindow is how close two clicks on the same row must be for the
// second to open its detail (matching linkDoubleClickWindow's feel).
const rowDoubleClickWindow = 400 * time.Millisecond

// rowOpensDetail reports whether a row kind has a detail/focus view that Tab
// opens and that single-click does NOT already open — i.e. one a double-click
// should enter (see handleReveal). A Wilds objective opens its PARENT quest.
// RowLabel is excluded: its name already opens the section on a single click.
func rowOpensDetail(k ui.RowKind) bool {
	switch k {
	case ui.RowQuest, ui.RowProject, ui.RowSection, ui.RowWildsObjective, ui.RowSagaLink:
		return true
	}
	return false
}

func (m *Model) clickRowAt(rows []ui.Row, idx int, msg tea.Mouse, colX int) tea.Cmd {
	if idx < 0 || idx >= len(rows) {
		return nil
	}
	row := rows[idx]
	if row.Kind == ui.RowSpacer {
		return nil
	}
	// A click on an integration code in a quest's meta sub-line opens its URL.
	// Meta rows aren't selectable, so this is handled before any cursor move.
	if row.Kind == ui.RowQuestMeta {
		for _, sp := range m.codeSpans[idx] {
			if msg.X >= sp.x0 && msg.X < sp.x1 {
				return openURL(sp.url)
			}
		}
		return nil
	}
	relX := msg.X - colX
	nestOffset := 0
	if row.Nested {
		nestOffset = 2
	}

	m.commitEdit()
	m.setCursor(row)
	// A click acts on the row it landed on — point the overlay origin at the
	// clicked cell so a completion burst (marking an objective/quest done) fires
	// THERE, not at the cursor's previous on-screen position. cursorScreen* is
	// otherwise only refreshed on the next render, which happens after the burst
	// is spawned. colX is the cursor-mark column; msg.Y is the clicked row.
	m.cursorScreenX = colX
	m.cursorScreenY = msg.Y

	// Double-click an openable row (quest / campaign / section) to enter its
	// detail view — the mouse equivalent of Tab.
	if rowOpensDetail(row.Kind) {
		now := time.Now()
		if m.cursor == m.lastRowClick && now.Sub(m.lastRowClickAt) < rowDoubleClickWindow {
			m.lastRowClick = cursorTarget{} // disarm so a third click doesn't re-open
			return m.handleReveal()
		}
		m.lastRowClick = m.cursor
		m.lastRowClickAt = now
	}

	// A click landing on a rendered action hint ("→ open (tab)", "↓ collapse
	// (enter)") triggers that action, exactly as pressing its key would.
	for _, sp := range m.hintSpans[idx] {
		if msg.X >= sp.x0 && msg.X < sp.x1 {
			switch sp.action {
			case "tab":
				return m.handleReveal()
			case "enter":
				return m.handleEnter()
			case "done":
				return m.toggleDone()
			case "rename":
				if row.Kind == ui.RowLookout {
					m.beginLookoutRename(row.QuestID, row.LookoutURL)
				}
				return nil
			case "copy":
				switch row.Kind {
				case ui.RowTrack:
					return m.copyTrack(row.QuestID, row.TrackEvent)
				case ui.RowRune:
					return m.copyToClipboard(ldFlagURL(m.ldProject, m.ldEnv, row.RuneKey), "link copied")
				case ui.RowLookout:
					return m.copyToClipboard(row.LookoutURL, "link copied")
				}
				return nil
			}
		}
	}

	if cmd, ok := m.commonRowClick(row); ok {
		return cmd
	}

	switch row.Kind {
	case ui.RowLabel:
		// Campaigns banner: chevron collapses all, name opens the focused view.
		if relX <= 1 {
			return m.handleEnter()
		}
		return m.handleReveal()
	case ui.RowProject, ui.RowSection:
		if relX <= 3+nestOffset {
			m.toggleReveal()
		} else if row.Kind == ui.RowProject {
			m.beginTextSelection(m.editor, relX-titleOffset(row, nestOffset))
		}
	case ui.RowQuest:
		if relX >= 4+nestOffset && relX <= 5+nestOffset {
			return m.toggleDone()
		}
		m.beginTextSelection(m.editor, relX-titleOffset(row, nestOffset))
	case ui.RowWildsObjective:
		// Mirror a quest row: only the checkbox glyph marks the objective done;
		// clicking the text edits it (double-click opens the parent quest, above).
		indent := m.objectiveIndent(row)
		checkX := wildsObjCol + 2*indent
		if relX >= checkX && relX <= checkX+1 {
			return m.toggleDone() // → markWildsObjectiveDone
		}
		m.beginTextSelection(m.editor, relX-objectiveTextOffset(indent))
	}
	return nil
}

// objectiveIndent is a Wilds objective row's nesting depth (its body line's
// Indent) — the objective shifts right 2 columns per level (see wildsObjCol),
// which the checkbox click zone and text caret both key off.
func (m *Model) objectiveIndent(row ui.Row) int {
	if q := m.findQuest(row.QuestID); q != nil {
		for _, l := range q.Body {
			if l.ID == row.BodyLineID {
				return l.Indent
			}
		}
	}
	return 0
}

// objectiveTextOffset is the column (from the column's content edge) where a
// Wilds objective's editable text starts: the checkbox slot (wildsObjCol, plus
// 2 per nest level) then the checkbox glyph and its trailing space.
func objectiveTextOffset(indent int) int {
	return wildsObjCol + 2*indent + 2
}

// beginTextSelection places ti's cursor at the clicked column (clamped to
// the text's bounds) and arms it as a selection anchor, so a subsequent
// drag (see dragTextSelection) extends a highlighted range from there —
// a plain click without dragging just repositions the cursor.
func (m *Model) beginTextSelection(ti *textinput.Model, relX int) {
	if ti == nil {
		return
	}
	runes := []rune(ti.Value())
	pos := clampInt(relX, 0, len(runes))
	ti.SetCursor(pos)
	m.selAnchor = pos
}

// dragTextSelection extends the row-title selection while the mouse moves
// with the left button held, as long as it's still over the same row the
// drag started on — moving onto a different row just stops updating
// rather than jumping the selection there.
func (m *Model) dragTextSelection(msg tea.Mouse, rows []ui.Row) tea.Cmd {
	if m.editor == nil || m.selAnchor == noSelection {
		return nil
	}
	relY := msg.Y - m.rowsScreenTop
	idx := m.scrollOffset + relY
	if idx < 0 || idx >= len(rows) || !m.cursor.matches(rows[idx]) {
		return nil
	}
	row := rows[idx]
	nestOffset := 0
	if row.Nested {
		nestOffset = 2
	}
	relX := msg.X - m.leftMargin - titleOffset(row, nestOffset)
	runes := []rune(m.editor.Value())
	m.editor.SetCursor(clampInt(relX, 0, len(runes)))
	return m.copySelection(m.editor)
}

// updateHover tracks which row the mouse is resting over (nil if none, or
// off the row area entirely) — purely for the "(tab: open)"/"(read only)"
// hints in View(); it never moves the cursor or changes any data.
func (m *Model) updateHover(msg tea.Mouse, rows []ui.Row) {
	relY := msg.Y - m.rowsScreenTop
	if relY < 0 {
		m.hover = nil
		return
	}
	idx := m.scrollOffset + relY
	if idx < 0 || idx >= len(rows) || rows[idx].Kind == ui.RowSpacer || rows[idx].Kind == ui.RowQuestMeta {
		m.hover = nil
		return
	}
	t := targetFromRow(rows[idx])
	m.hover = &t
}
