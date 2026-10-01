package app

import (
	"sort"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The Trails room is the Tavern's open-PR overview: every un-vaulted quest's
// linked PRs that aren't merged/closed, grouped by quest and ordered into their
// stack (Graphite / gh), each showing live CI / draft / comment status so the
// ones needing attention — failing CI or unresolved comments — read at a glance.
// PR status lives in the app (m.prStatus), not the store, so this room is built
// app-side rather than in the ui package like Runes/Lookouts.

// prOpen reports whether a linked PR belongs in the Trails room: not confirmed
// merged or closed. A PR whose status hasn't synced yet counts as open — we
// don't hide it until we know it's settled.
func (m *Model) prOpen(code string) bool {
	st, ok := m.prStatus[code]
	if !ok {
		return true
	}
	return st.Status != "merged" && st.Status != "closed"
}

// prNeedsAttention reports whether an open PR is asking for help — failing CI or
// unresolved review comments — used to float such quests to the top of the room.
func (m *Model) prNeedsAttention(code string) bool {
	st, ok := m.prStatus[code]
	if !ok {
		return false
	}
	return st.Status == "error" || st.CommentsTotal > st.CommentsResolved
}

// openPRLinks is a quest's linked PRs filtered to the still-open ones, in the
// quest's stored order.
func (m *Model) openPRLinks(q *model.Quest) []model.PRLink {
	var out []model.PRLink
	for _, pr := range q.PRs {
		if m.prOpen(pr.Code) {
			out = append(out, pr)
		}
	}
	return out
}

// trailsCount is the number of OPEN PRs across un-vaulted quests — the Trails
// room's badge (what still needs checking).
func (m *Model) trailsCount() int {
	n := 0
	for i := range m.store.Quests {
		if q := &m.store.Quests[i]; !m.isVaulted(q) {
			n += len(m.openPRLinks(q))
		}
	}
	return n
}

// hasAnyTrails reports whether any un-vaulted quest has a linked PR at all
// (merged or not). The room's visibility keys off this, not the open count, so
// it doesn't vanish the moment your last open PR merges — it stays as a stable
// daily-check surface and just shows an empty state until a new PR appears.
func (m *Model) hasAnyTrails() bool {
	for i := range m.store.Quests {
		if q := &m.store.Quests[i]; !m.isVaulted(q) && len(q.PRs) > 0 {
			return true
		}
	}
	return false
}

// isTrailResyncRow reports whether "r" on this row kind resyncs the owning
// quest's trails — the Trails/Runes/Lookouts quest headers and their found-from-
// trails items. RowLookout is excluded (its "r" renames the dashboard).
func isTrailResyncRow(k ui.RowKind) bool {
	switch k {
	case ui.RowTrailQuest, ui.RowRuneQuest, ui.RowLookoutQuest, ui.RowTrail, ui.RowRune, ui.RowTrack:
		return true
	}
	return false
}

// isTrailResyncSection reports whether a quest-detail section is found from
// trails — Trails, Runes, Tracks — so its header carries the resync affordance.
func isTrailResyncSection(sectionKey string) bool {
	return sectionKey == secTrails || sectionKey == secRunes || sectionKey == secTracks
}

// roomQuestTitle renders a quest-group header for the Trails / Runes / Lookouts
// rooms identically: the title, plus the resync affordance (or a spinner while
// resyncing) when it's selected or mid-resync — shown only for quests that have
// trails to resync (runes/tracks are found from PRs, so a quest with them has
// PRs; a dashboard-only quest doesn't, and gets no resync control).
func (m *Model) roomQuestTitle(row ui.Row, isCursor bool) string {
	name := ui.StyleName.Render(row.Label)
	if isCursor {
		name = ui.StyleTitle.Render(row.Label)
	}
	if q := m.findQuest(row.QuestID); q != nil && len(q.PRs) > 0 && (isCursor || m.findingQuestID == row.QuestID) {
		name += "  " + m.resyncAffordance(row.QuestID)
	}
	return name
}

// trailsRows builds the Trails room pane (header-less, like ui.SectionContent):
// one collapsible header per un-vaulted quest with open PRs, then that quest's
// PRs in stack order with tree connectors on real stacks. Quests with an
// attention-needed PR sort first, then by title, so trouble surfaces at the top.
func (m *Model) trailsRows() []ui.Row {
	type group struct {
		q         *model.Quest
		prs       []model.PRLink
		attention bool
	}
	var groups []group
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		if m.isVaulted(q) {
			continue
		}
		open := m.openPRLinks(q)
		if len(open) == 0 {
			continue
		}
		att := false
		for _, pr := range open {
			if m.prNeedsAttention(pr.Code) {
				att = true
				break
			}
		}
		groups = append(groups, group{q: q, prs: open, attention: att})
	}
	sort.SliceStable(groups, func(a, b int) bool {
		if groups[a].attention != groups[b].attention {
			return groups[a].attention // attention-needed first
		}
		return groups[a].q.Title < groups[b].q.Title
	})

	if len(groups) == 0 {
		// The room is present (you have PRs) but none are open — a calm all-clear.
		return []ui.Row{{Kind: ui.RowDayHeader, Label: "No open trails — the road is clear."}}
	}
	var rows []ui.Row
	first := true
	for _, g := range groups {
		// One header per quest (not collapsible — the whole point of the room is to
		// see every open PR at a glance; the "synced Xs ago" freshness rides on the
		// header's resync affordance), then its PRs in stack order.
		if !first {
			rows = append(rows, ui.Row{Kind: ui.RowSpacer})
		}
		first = false
		rows = append(rows, ui.Row{Kind: ui.RowTrailQuest, QuestID: g.q.ID, Label: g.q.Title})
		// Order like the quest detail: top of the stack first, "└ main" closing it.
		for _, d := range stackDisplay(m.prStack(g.prs)) {
			if d.isMain {
				rows = append(rows, ui.Row{Kind: ui.RowDayHeader, Label: d.marker + " main"})
				continue
			}
			rows = append(rows, ui.Row{
				Kind:    ui.RowTrail,
				QuestID: g.q.ID,
				Code:    d.node.link.Code,
				Repo:    d.node.link.Repo,
				Label:   d.marker,
			})
		}
	}
	return rows
}

// trailRowContent renders one PR row's live status (everything after the stack
// gutter): its state glyph, the code, then a muted "[draft · ]<ci word> ·
// <resolved>/<total> comments".
func (m *Model) trailRowContent(code string) string {
	glyph, _ := m.prGlyph(code)
	status := m.prStatusWord(code)
	if st, ok := m.prStatus[code]; ok && st.Draft {
		status = "draft · " + status
	}
	status += " · " + m.prReviewBadges(code)
	return glyph + " " + ui.StyleName.Render(code) + "  " + ui.StyleMuted.Render(status)
}
