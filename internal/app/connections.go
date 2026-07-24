package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

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
func connStyle(c lipgloss.TerminalColor) lipgloss.Style {
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
	switch m.workspaceState(id) {
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
	styleOr := func(s lipgloss.Style) lipgloss.Style {
		if silent {
			return ui.StyleMuted
		}
		return s
	}
	var icons []string
	for _, id := range q.AgentWorkspaces {
		icons = append(icons, styleOr(m.agentConnStyle(id)).Render(ui.GlyphConnNPC))
	}
	for _, code := range q.JiraCodes {
		icons = append(icons, styleOr(m.jiraConnStyle(code)).Render(ui.GlyphConnScroll))
	}
	for _, pr := range q.PRs {
		icons = append(icons, styleOr(m.prConnStyle(pr.Code)).Render(ui.GlyphConnTrail))
	}
	for _, key := range q.Runes {
		icons = append(icons, styleOr(m.runeConnStyle(key)).Render(ui.GlyphConnRune))
	}
	if len(icons) == 0 {
		return ""
	}
	return "  " + strings.Join(icons, " ")
}
