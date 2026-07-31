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
	// While the search bar is open, it owns its keys (typing, facet cycling,
	// close); everything else falls through to the normal handling below so
	// you can still navigate and act on the filtered results.
	if m.searchOpen {
		if cmd, handled := m.handleSearchBarKey(msg); handled {
			return cmd
		}
	}
	switch {
	case key.Matches(msg, Keys.Help):
		m.commitEdit()
		m.pushModal(helpModal())
		return nil
	case key.Matches(msg, Keys.Search):
		m.openSearch()
		return nil
	case key.Matches(msg, Keys.ToggleHints):
		m.hideHoverTips = !m.hideHoverTips
		return nil
	case key.Matches(msg, Keys.Undo):
		m.undo()
		return nil
	case key.Matches(msg, Keys.SetOut):
		return m.setWilds(!m.wilds)
	case msg.Code == tea.KeyEsc && m.wilds:
		return m.setWilds(false)
	// Two-column Tavern: jump to a section (Ctrl+1..4, in the rail's visual
	// order — Questboard/Runes/Vault top-to-bottom, then Campaigns) or switch
	// columns with Left/Right once the caret is at the title's edge (otherwise
	// the arrow keeps editing text).
	case m.twoColumn() && msg.String() == "ctrl+1":
		m.jumpToSection("inbox")
		return nil
	case m.twoColumn() && msg.String() == "ctrl+2":
		m.jumpToSection("runes")
		return nil
	case m.twoColumn() && msg.String() == "ctrl+3":
		m.jumpToSection("someday")
		return nil
	case m.twoColumn() && msg.String() == "ctrl+4":
		m.jumpToSection("campaigns")
		return nil
	// Ctrl+Shift+1..3 collapse/expand a rail section in place (Campaigns isn't a
	// collapsible box, so there's no Ctrl+Shift+4).
	case m.twoColumn() && msg.String() == "ctrl+shift+1":
		m.toggleSectionCollapse("inbox")
		return nil
	case m.twoColumn() && msg.String() == "ctrl+shift+2":
		m.toggleSectionCollapse("runes")
		return nil
	case m.twoColumn() && msg.String() == "ctrl+shift+3":
		m.toggleSectionCollapse("someday")
		return nil
	// The rail is the left column, campaigns the right: Left→rail, Right→campaigns.
	case m.twoColumn() && key.Matches(msg, Keys.Left) && m.caretAtStart():
		m.switchColumn(true)
		return nil
	case m.twoColumn() && key.Matches(msg, Keys.Right) && m.caretAtEnd():
		m.switchColumn(false)
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
		for i := 0; i < half; i++ {
			m.moveCursor(delta)
		}
		return nil
	}
	return m.handleRowKey(msg)
}

// handleRowKey handles every action keyed off whatever m.cursor currently
// targets — shared by the main outline and a focused campaign's quest list
// (see updateModal's ModalCampaignDetail case), so Tab/Enter/Ctrl+D/Ctrl+X/
// etc. behave identically in both places.
func (m *Model) handleRowKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.confirmDeleteID != "" {
		target := m.cursor
		id := m.confirmDeleteID
		m.confirmDeleteID = ""
		if msg.String() == "y" {
			switch target.kind {
			case ui.RowQuest:
				m.removeCurrentRow(func() { m.deleteQuestByID(id) })
			case ui.RowProject:
				m.removeCurrentRow(func() { m.deleteProjectByID(id) })
			case ui.RowRune:
				qid := target.questID
				m.removeCurrentRow(func() { m.detachRuneFromQuest(qid, id) })
			}
		}
		return nil
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
	case key.Matches(msg, Keys.Delete):
		m.openConfirmDelete()
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

// moveCursor steps to the previous/next row, skipping over spacer/label rows
// (they're purely visual and never a valid cursor target).
func (m *Model) moveCursor(delta int) {
	rows := m.visibleRows()
	if len(rows) == 0 {
		return
	}
	m.commitEdit()
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
	// Drop a fading ghost where the cursor was, then move it instantly — a
	// trail behind the cursor, no easing delay. cursorScreen{X,Y} still holds
	// the last-rendered (old) cell here.
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
	case ui.RowSection:
		m.collapsedSections[m.cursor.section] = !m.collapsedSections[m.cursor.section]
		m.saveLayoutConfig()
	case ui.RowRuneQuest:
		// Rune-quest groups reuse collapsedProjects keyed by quest ID.
		m.collapsedProjects[m.cursor.questID] = !m.collapsedProjects[m.cursor.questID]
	}
	m.invalidateRender()
}

// handleReveal is Tab / a section-name click: opens a focused, full-screen
// detail view for a campaign, a quest, or a section. Every section
// (Questboard / Runes / Campaigns / Vault) has one; the "Campaigns" label opens
// the campaigns section view.
func (m *Model) handleReveal() tea.Cmd {
	switch m.cursor.kind {
	case ui.RowProject:
		m.commitEdit()
		if p := m.findProject(m.cursor.projectID); p != nil {
			// The focused quest sublist navigates via the global outline
			// (see handleEnter's RowNewQuest/RowQuest cases), so the
			// campaign needs to already be expanded there.
			m.collapsedProjects[p.ID] = false
			m.pushModal(campaignDetailModal(p))
		}
	case ui.RowQuest:
		m.commitEdit()
		if q := m.findQuest(m.cursor.questID); q != nil {
			m.pushModal(questDetailModal(q))
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
	rows := m.currentRowScope()
	idx := findRowIndex(rows, m.cursor)
	if idx < 0 {
		return nil
	}
	row := rows[idx]
	m.commitEdit()

	switch row.Kind {
	case ui.RowNewProject:
		p := model.Project{ID: store.NewID(), Name: ""}
		m.store.Projects = append(m.store.Projects, p)
		m.save()
		m.setCursor(ui.Row{Kind: ui.RowProject, ProjectID: p.ID})

	case ui.RowNewQuest:
		if row.ProjectID != "" {
			p := m.findProject(row.ProjectID)
			if p == nil || p.Archived {
				return nil
			}
			m.collapsedProjects[row.ProjectID] = false
		}
		q := m.newQuestUnder(row.ProjectID, model.StatusOpen)
		m.setCursor(ui.Row{Kind: ui.RowQuest, ProjectID: q.ProjectID, QuestID: q.ID})

	case ui.RowProject, ui.RowSection, ui.RowRuneQuest:
		m.toggleReveal()

	case ui.RowRune:
		return m.openConnection(connection{kind: linkRune, code: row.RuneKey, url: ldFlagURL(m.ldProject, m.ldEnv, row.RuneKey)})

	case ui.RowNewRune:
		m.openRunePicker("")

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
// the quest has details (a non-empty body), it's not deleted silently —
// this arms an inline y/n confirmation instead (see confirmDeleteID), since
// the empty title could otherwise hide real data being lost. Deleting a
// project this way requires it to already be empty of quests; otherwise
// Ctrl+X (with confirmation) is required.
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
				m.confirmDeleteID = id
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
func (m *Model) toggleDone() tea.Cmd {
	if m.cursor.kind == ui.RowWildsObjective {
		return m.markWildsObjectiveDone()
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
	becameDone := false
	if q.Status == model.StatusDone {
		q.Status = model.StatusOpen
		q.CompletedAt = nil
	} else {
		q.Status = model.StatusDone
		now := time.Now()
		q.CompletedAt = &now
		becameDone = true
	}
	q.UpdatedAt = time.Now()
	m.reslotIfTierChanged(q.ID, before)
	m.save()
	if becameDone {
		// A quest gets the ring "seal stamp"; objectives get the scatter burst.
		m.spawnSparkleRing(m.cursorScreenX+questGlyphCol, m.cursorScreenY, 12)
		return tea.Batch(m.playSound(sndQuestDone), m.maybeStartOverlayTick())
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
	if q == nil || q.InQuestboard() {
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
	if m.cursor.kind != ui.RowQuest {
		return nil
	}
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
	return nil
}

func (m *Model) openProjectPicker() {
	if m.cursor.kind != ui.RowQuest {
		return
	}
	q := m.findQuest(m.cursor.questID)
	if q == nil {
		return
	}
	m.commitEdit()
	srcIdx := findRowIndex(m.currentRowScope(), m.cursor)
	mod := projectPickerModal(m.store, q.ID, q.ProjectID)
	mod.SourceRowIdx = srcIdx
	m.pushModal(mod)
}

// openConfirmDelete arms the same lightweight inline y/n prompt for whatever
// is under the cursor — a quest or a campaign (which cascades to every
// quest inside it). No popup for either; see confirmDeleteID.
func (m *Model) openConfirmDelete() {
	m.commitEdit()
	switch m.cursor.kind {
	case ui.RowQuest:
		if m.findQuest(m.cursor.questID) == nil {
			return
		}
		m.confirmDeleteID = m.cursor.questID
	case ui.RowProject:
		if m.findProject(m.cursor.projectID) == nil {
			return
		}
		m.confirmDeleteID = m.cursor.projectID
	case ui.RowRune:
		m.confirmDeleteID = m.cursor.runeKey
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
	mouse := msg.Mouse()
	m.hoverSection = ""
	m.confirmDeleteID = "" // any click/scroll cancels a pending inline delete confirm

	if mouse.Button != tea.MouseLeft {
		return nil
	}

	// A divider press starts a resize drag instead of any row/title dispatch
	// below — see tavern_columns.go's hitTestDivider.
	if cmd, started := m.tryStartResizeDrag(mouse.X, mouse.Y); started {
		return cmd
	}

	rows := m.visibleRows()
	if len(rows) == 0 {
		return nil
	}

	// A click on a TAVERN/WILDS header label switches to that mode.
	if mouse.Y == m.modeToggleRow {
		for _, sp := range m.modeSpans {
			if mouse.X >= sp.x0 && mouse.X < sp.x1 {
				return m.setWilds(sp.wilds)
			}
		}
		return nil
	}

	// Two-column Tavern clicks are mapped by the per-column line→row tables the
	// renderer built (boxes shift lines, so plain arithmetic won't do).
	if m.twoColumn() {
		return m.handleTwoColumnClick(mouse)
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
	mouse := msg.Mouse()
	m.confirmDeleteID = ""
	delta := 1
	if mouse.Button == tea.MouseWheelUp {
		delta = -1
	}
	// In the two-column Tavern the wheel scrolls whichever section it's over
	// (its own scroll view), moving the cursor only when that section holds
	// it. Elsewhere it moves the cursor as before.
	if m.twoColumn() {
		m.wheelSection(m.sectionAtPoint(mouse), delta)
		return nil
	}
	m.moveCursor(delta)
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
	if m.twoColumn() && !m.leftDown {
		m.updateResizeHover(mouse.X, mouse.Y)
	}
	rows := m.visibleRows()
	if len(rows) == 0 {
		return nil
	}
	if m.leftDown {
		return m.dragTextSelection(mouse, rows)
	}
	if m.twoColumn() {
		m.updateSectionHover(mouse)
		return nil
	}
	m.updateHover(mouse, rows)
	return nil
}

// updateResizeHover tracks which divider (if any) the mouse currently rests
// on, purely for the hover line/joint highlight — never gates whether a drag
// can start (tryStartResizeDrag hit-tests fresh at press time).
func (m *Model) updateResizeHover(x, y int) {
	next := m.hitTestDivider(x, y)
	if next == m.resizeHover {
		return
	}
	m.resizeHover = next
	m.invalidateRender()
}

// commonRowClick performs the click actions that are identical on every list
// surface — opening a rune, checking off a Wilds objective, collapsing a
// rune-quest group, or adding via a "+ New …" affordance. It returns
// (cmd, true) when it handled the row kind; (nil, false) leaves the
// surface-specific kinds (Section chevron, Quest checkbox — whose click zones
// depend on that surface's layout) to the caller. One definition of what a
// click does to these rows, so the Tavern rail and campaigns/Wilds column
// can't drift apart (the kind of split that hid the rune-click bug).
func (m *Model) commonRowClick(row ui.Row) (tea.Cmd, bool) {
	switch row.Kind {
	case ui.RowRune:
		return m.openConnection(connection{kind: linkRune, code: row.RuneKey, url: ldFlagURL(m.ldProject, m.ldEnv, row.RuneKey)}), true
	case ui.RowRuneQuest:
		m.toggleReveal()
		return nil, true
	case ui.RowWildsObjective:
		return m.toggleDone(), true
	case ui.RowNewProject, ui.RowNewQuest, ui.RowNewRune:
		return m.handleEnter(), true
	}
	return nil, false
}

// clickRowAt handles a left-press on the row at idx of rows: opening a meta
// line's code, moving the cursor, firing a clicked hint, or the per-kind
// default (toggle collapse/done, open a rune, add). colX is the column's
// content left edge — relX is measured from it, matching how hint/code spans
// were recorded.
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
	}
	return nil
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
