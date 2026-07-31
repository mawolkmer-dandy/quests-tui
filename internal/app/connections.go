package app

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// connection is one of a quest's links to another system — the single model
// both surfaces derive from (the Tavern's compact emblems and the detail
// page's "Sigils" list), so status and open behavior can't drift apart. Each
// surface still presents it in its own visual language, but the source data,
// status styling, and "open" action are shared.
type connection struct {
	kind linkKind // linkAgent | linkJira | linkPR | linkRune
	code string   // agent terminal id / Jira code / PR number / rune key
	repo string   // PR only
	url  string   // browser URL; empty for agents (they focus their pane instead)
}

// questConnections lists a quest's connections in canonical order: NPCs
// (agents), Scrolls (Jira), Trails (PRs), Runes (flags).
func (m *Model) questConnections(q *model.Quest) []connection {
	var cs []connection
	for _, id := range q.AgentWorkspaces {
		cs = append(cs, connection{kind: linkAgent, code: id})
	}
	for _, code := range q.JiraCodes {
		cs = append(cs, connection{kind: linkJira, code: code, url: jiraURL(code, m.jiraBaseURL)})
	}
	for _, pr := range q.PRs {
		cs = append(cs, connection{kind: linkPR, code: pr.Code, repo: pr.Repo, url: prURL(pr.Repo, pr.Code)})
	}
	for _, key := range q.Runes {
		cs = append(cs, connection{kind: linkRune, code: key, url: ldFlagURL(m.ldProject, m.ldEnv, key)})
	}
	return cs
}

// openConnection activates a connection — focus a pinned agent's pane, or open
// the link in the browser. The single definition of "open a connection", so
// every surface behaves identically.
func (m *Model) openConnection(c connection) tea.Cmd {
	if c.kind == linkAgent {
		return openAgent(c.code)
	}
	return openURL(c.url)
}

// connEmblem is the Tavern's compact per-kind emblem glyph.
func connEmblem(kind linkKind) string {
	switch kind {
	case linkAgent:
		return ui.GlyphConnNPC
	case linkJira:
		return ui.GlyphConnScroll
	case linkPR:
		return ui.GlyphConnTrail
	case linkRune:
		return ui.GlyphConnRune
	}
	return ""
}

// connStatusStyle is the faint status color for a connection, dispatching to
// the per-kind status mapping — one entry point instead of four scattered
// call sites.
func (m *Model) connStatusStyle(c connection) lipgloss.Style {
	switch c.kind {
	case linkAgent:
		return m.agentConnStyle(c.code)
	case linkJira:
		return m.jiraConnStyle(c.code)
	case linkPR:
		return m.prConnStyle(c.code)
	case linkRune:
		return m.runeConnStyle(c.code)
	}
	return ui.StyleMuted
}

// A quest's "connections" are its links to other systems, each with a
// gamified identity (name + emblem):
//
//	NPC     — a pinned Claude agent    (☗)
//	Scrolls — a Jira spec              (§)
//	Trails  — a GitHub pull request    (⎇)
//	Runes   — a LaunchDarkly flag      (◈)
//
// In the main list each connection shows as a single emblem after the quest
// title, colored by its status (green good / yellow in-flight / red needs
// attention / purple merged / muted loading). Colors are deliberately faint so
// they read as a glance, not a shout. A vaulted quest's connections go
// "silent" — muted and unfetched (see collectSyncTargets / connectionIcons).

// connStyle is a faint (muted-saturation) colored style for a connection glyph.
func connStyle(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Faint(true).Foreground(c)
}

// jiraConnStyle colors a Scroll: backlog muted, in-progress yellow, done green.
func (m *Model) jiraConnStyle(code string) lipgloss.Style {
	st, ok := m.jiraStatus[code]
	if !ok {
		return ui.StyleMuted
	}
	switch st.Status {
	case "done":
		return connStyle(ui.ColorHeading)
	case "in progress":
		return connStyle(ui.ColorPriorityMedium)
	default:
		return ui.StyleMuted // todo / backlog
	}
}

// prConnStyle colors a Trail: merged purple, failed CI or unresolved comments
// red, in-flight yellow, passing-and-clean green, closed/loading muted.
func (m *Model) prConnStyle(code string) lipgloss.Style {
	st, ok := m.prStatus[code]
	if !ok {
		return ui.StyleMuted
	}
	switch st.Status {
	case "merged":
		return connStyle(ui.ColorMerged)
	case "closed":
		return ui.StyleMuted
	case "error":
		return connStyle(ui.ColorImportant)
	case "running":
		return connStyle(ui.ColorPriorityMedium)
	default: // success
		if st.CommentsTotal > st.CommentsResolved {
			return connStyle(ui.ColorImportant) // unresolved comment needs attention
		}
		return connStyle(ui.ColorHeading)
	}
}

// runeConnStyle colors a Rune: off in both envs muted, on in one yellow, on in
// both green.
func (m *Model) runeConnStyle(key string) lipgloss.Style {
	st, ok := m.runeStatus[key]
	if !ok {
		return ui.StyleMuted
	}
	on := func(s string) bool { return s == "on" || s == "partial" }
	switch {
	case on(st.State) && on(st.StateTest):
		return connStyle(ui.ColorHeading)
	case on(st.State) || on(st.StateTest):
		return connStyle(ui.ColorPriorityMedium)
	default:
		return ui.StyleMuted
	}
}

// agentConnStyle colors an NPC by its live herdr status.
func (m *Model) agentConnStyle(id string) lipgloss.Style {
	switch m.agentState(id) {
	case "blocked":
		return connStyle(ui.ColorImportant)
	case "working":
		return connStyle(ui.ColorPriorityMedium)
	case "idle":
		return connStyle(ui.ColorHeading)
	default:
		return ui.StyleMuted
	}
}

// connectionIcons is the compact string of status-colored emblems shown after
// a quest's title in the list — one per connection (NPCs, Scrolls, Trails,
// Runes, in that order). A vaulted quest's emblems are all muted (silent).
func (m *Model) connectionIcons(q *model.Quest) string {
	silent := m.isVaulted(q)
	var icons []string
	for _, c := range m.questConnections(q) {
		st := m.connStatusStyle(c)
		if silent {
			st = ui.StyleMuted // a vaulted quest's emblems go quiet
		}
		icons = append(icons, st.Render(connEmblem(c.kind)))
	}
	if len(icons) == 0 {
		return ""
	}
	return "  " + strings.Join(icons, " ")
}
