package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Claude-agent integration. `claude agents --json` lists currently-tracked
// sessions with their cwd (usually a git worktree), name, and live
// status/state. A quest pins worktrees (Quest.AgentWorktrees); any agent whose
// cwd is a pinned worktree shows its live state on the quest, like the Jira/PR
// integrations.

// AgentInfo is one entry of `claude agents --json`.
type AgentInfo struct {
	Cwd       string `json:"cwd"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Status    string `json:"status"` // "busy" | "idle" | "waiting"
	State     string `json:"state"`  // "working" | "done" | "blocked" | "paused" (background only)
	Kind      string `json:"kind"`   // "background" | "interactive"
}

// agentsMsg carries a refreshed agent list into Update (from refreshAgentsCmd,
// fired immediately after pinning; the periodic sync updates m.agents too).
type agentsMsg struct{ agents []AgentInfo }

// fetchAgents runs `claude agents --json` and parses the currently-tracked
// sessions. ok is false when the command fails (claude missing / not logged in)
// so a transient error doesn't wipe the cached list.
func fetchAgents() (agents []AgentInfo, ok bool) {
	out, err := runCmd("claude", "agents", "--json")
	if err != nil {
		return nil, false
	}
	if err := json.Unmarshal(out, &agents); err != nil {
		return nil, false
	}
	return agents, true
}

// refreshAgentsCmd fetches the agent list off the UI goroutine, for an
// immediate refresh right after pinning (independent of the sync ticker).
func refreshAgentsCmd() tea.Cmd {
	return func() tea.Msg {
		agents, ok := fetchAgents()
		if !ok {
			return nil
		}
		return agentsMsg{agents: agents}
	}
}

// hasAgentLinks reports whether any quest pins a worktree, so the sync loop
// keeps refreshing agent state even when nothing else needs syncing.
func (m *Model) hasAgentLinks() bool {
	for i := range m.store.Quests {
		if len(m.store.Quests[i].AgentWorktrees) > 0 {
			return true
		}
	}
	return false
}

// worktreeAgents returns the agents whose cwd is exactly worktree. Matching is
// exact, not prefix-based: a prefix match on the repo root would swallow every
// worktree under .claude/worktrees/, mislabeling the pin.
func (m *Model) worktreeAgents(worktree string) []AgentInfo {
	var out []AgentInfo
	for _, a := range m.agents {
		if a.Cwd == worktree {
			out = append(out, a)
		}
	}
	return out
}

// agentEffectiveState collapses one agent's status+state into a single label:
// blocked (needs input) / working / paused / done / idle.
func agentEffectiveState(a AgentInfo) string {
	switch a.State {
	case "blocked":
		return "blocked"
	case "working":
		return "working"
	case "paused":
		return "paused"
	case "done":
		return "done"
	}
	switch a.Status {
	case "waiting":
		return "blocked"
	case "busy":
		return "working"
	}
	return "idle"
}

// agentStatePriority orders states by how much they want attention, so a
// worktree with several agents surfaces the most important one.
func agentStatePriority(state string) int {
	switch state {
	case "blocked":
		return 5
	case "working":
		return 4
	case "idle":
		return 3
	case "paused":
		return 2
	case "done":
		return 1
	}
	return 0
}

// worktreeState is the single state shown for a pinned worktree: the
// highest-priority state among its agents, or "none" when nothing runs there.
func (m *Model) worktreeState(worktree string) string {
	best, bestPrio := "none", 0
	for _, a := range m.worktreeAgents(worktree) {
		s := agentEffectiveState(a)
		if p := agentStatePriority(s); p > bestPrio {
			best, bestPrio = s, p
		}
	}
	return best
}

// worktreeLabel is the display name for a pinned worktree's line: the name of
// its highest-priority agent, else the worktree's folder name.
func (m *Model) worktreeLabel(worktree string) string {
	name, bestPrio := "", 0
	for _, a := range m.worktreeAgents(worktree) {
		if a.Name == "" {
			continue
		}
		if p := agentStatePriority(agentEffectiveState(a)); p >= bestPrio {
			name, bestPrio = a.Name, p
		}
	}
	if name != "" {
		return name
	}
	return filepath.Base(worktree)
}

// agentGlyph is the state-colored status icon for an agent/worktree state.
func agentGlyph(state string) string {
	switch state {
	case "blocked":
		return lipgloss.NewStyle().Foreground(ui.ColorImportant).Render(ui.GlyphAgentBlocked)
	case "working":
		return ui.StyleRunning.Render(ui.GlyphAgentWorking)
	case "idle", "done":
		return lipgloss.NewStyle().Foreground(ui.ColorHeading).Render(ui.GlyphAgentIdle)
	case "paused":
		return ui.StyleMuted.Render(ui.GlyphAgentPaused)
	default: // none
		return ui.StyleMuted.Render(ui.GlyphAgentNone)
	}
}

// agentWord is the status word shown beside the icon.
func agentWord(state string) string {
	switch state {
	case "blocked":
		return "blocked"
	case "working":
		return "working"
	case "done":
		return "done"
	case "paused":
		return "paused"
	case "idle":
		return "idle"
	default:
		return "no session"
	}
}

// openAgentPicker opens the picker to pin another worktree/agent to the focused
// quest. A no-op outside a quest detail view.
func (m *Model) openAgentPicker() {
	if m.modal == nil || m.modal.Kind != ModalQuestDetail {
		return
	}
	q := m.findQuest(m.modal.QuestID)
	if q == nil {
		return
	}
	// Fetch fresh so the picker is populated even before the sync loop has run.
	if agents, ok := fetchAgents(); ok {
		m.agents = agents
	}
	m.commitBodyLine()
	m.clearFocusLink()
	m.pushModal(&Modal{Kind: ModalAgentPicker, TargetQuestID: q.ID, PickerItems: agentPickerItems(m.agents)})
}

// agentPickerItems lists the currently-tracked agents for the picker, one per
// worktree (deduped by cwd), labelled "<name> (<worktree>) · <state>". The ID
// is the cwd — what gets pinned to the quest.
func agentPickerItems(agents []AgentInfo) []pickerItem {
	seen := map[string]bool{}
	var items []pickerItem
	for _, a := range agents {
		if a.Cwd == "" || seen[a.Cwd] {
			continue
		}
		seen[a.Cwd] = true
		name := a.Name
		if name == "" {
			name = filepath.Base(a.Cwd)
		}
		label := fmt.Sprintf("%s  (%s) · %s", name, filepath.Base(a.Cwd), agentWord(agentEffectiveState(a)))
		items = append(items, pickerItem{ID: a.Cwd, Label: label})
	}
	return items
}
