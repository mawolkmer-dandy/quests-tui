package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Claude-agent integration. `claude agents --json` lists currently-tracked
// sessions with their cwd (usually a git worktree), sessionId, name, and live
// status/state. A quest pins one worktree (Quest.AgentWorktree); any agent
// whose cwd is that worktree is "this quest's agent", shown like the Jira/PR
// integrations and reopenable with `claude --resume`.

// AgentInfo is one entry of `claude agents --json`.
type AgentInfo struct {
	Cwd       string `json:"cwd"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Status    string `json:"status"` // "busy" | "idle"
	State     string `json:"state"`  // "working" | "done" (background agents only)
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

// refreshAgentsCmd fetches the agent list off the UI goroutine. Used for an
// immediate refresh right after pinning a worktree, independent of the sync
// ticker's overlap guard.
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
		if m.store.Quests[i].AgentWorktree != "" {
			return true
		}
	}
	return false
}

// questAgents returns the agents whose cwd is the quest's pinned worktree (or a
// subdirectory of it).
func (m *Model) questAgents(q *model.Quest) []AgentInfo {
	if q.AgentWorktree == "" {
		return nil
	}
	var out []AgentInfo
	for _, a := range m.agents {
		if a.Cwd == q.AgentWorktree || strings.HasPrefix(a.Cwd, q.AgentWorktree+string(filepath.Separator)) {
			out = append(out, a)
		}
	}
	return out
}

// agentEffectiveState collapses an agent's status+state into one of
// "working" / "done" / "idle".
func agentEffectiveState(a AgentInfo) string {
	if a.State == "working" || a.Status == "busy" {
		return "working"
	}
	if a.State == "done" {
		return "done"
	}
	return "idle"
}

// questAgentState aggregates the pinned worktree's agents into one state:
// working if any is working, else idle if any is idle, else done — or "none"
// when the worktree currently has no tracked agent.
func (m *Model) questAgentState(q *model.Quest) string {
	agents := m.questAgents(q)
	if len(agents) == 0 {
		return "none"
	}
	state := "done"
	for _, a := range agents {
		switch agentEffectiveState(a) {
		case "working":
			return "working"
		case "idle":
			state = "idle"
		}
	}
	return state
}

// questAgentLabel is the display name for the quest's agent line: a working
// agent's name wins, else the first agent's, else the worktree's folder name
// (when nothing is running there right now).
func (m *Model) questAgentLabel(q *model.Quest) string {
	agents := m.questAgents(q)
	if len(agents) == 0 {
		return filepath.Base(q.AgentWorktree)
	}
	for _, a := range agents {
		if agentEffectiveState(a) == "working" && a.Name != "" {
			return a.Name
		}
	}
	if agents[0].Name != "" {
		return agents[0].Name
	}
	return filepath.Base(q.AgentWorktree)
}

// agentGlyph is the state-colored spark for an agent state.
func agentGlyph(state string) string {
	switch state {
	case "working":
		return ui.StyleRunning.Render(ui.GlyphAgentWorking)
	case "done":
		return lipgloss.NewStyle().Foreground(ui.ColorHeading).Render(ui.GlyphAgentDone)
	case "idle":
		return ui.StyleMuted.Render(ui.GlyphAgentIdle)
	default:
		return ui.StyleMuted.Render(ui.GlyphAgentNone)
	}
}

// agentWord is the expanded-view status word for an agent state.
func agentWord(state string) string {
	switch state {
	case "working":
		return "working"
	case "done":
		return "done"
	case "idle":
		return "idle"
	default:
		return "no session"
	}
}

// openAgentSession reopens the quest's pinned session: a live session id is
// resumed (`claude --resume`), otherwise the most recent conversation in the
// worktree is continued (`claude --continue`). It opens in a new tmux split
// when running under tmux, else a new Terminal window.
func (m *Model) openAgentSession(q *model.Quest) tea.Cmd {
	if q.AgentWorktree == "" {
		return nil
	}
	cwd := q.AgentWorktree
	sessionID := ""
	for _, a := range m.questAgents(q) {
		if a.SessionID == "" {
			continue
		}
		sessionID = a.SessionID
		if agentEffectiveState(a) == "working" {
			break // prefer the actively-working session
		}
	}
	return openClaudeSession(cwd, sessionID)
}

func openClaudeSession(cwd, sessionID string) tea.Cmd {
	return func() tea.Msg {
		claudeBin, err := exec.LookPath("claude")
		if err != nil {
			claudeBin = "claude"
		}
		args := []string{"--continue"}
		if sessionID != "" {
			args = []string{"--resume", sessionID}
		}

		if os.Getenv("TMUX") != "" {
			// tmux execs the command directly (no shell), so args pass cleanly.
			tmuxArgs := append([]string{"split-window", "-h", "-c", cwd, claudeBin}, args...)
			_ = exec.Command("tmux", tmuxArgs...).Start()
			return nil
		}

		// No tmux: open a new Terminal window running the command in the worktree.
		inner := fmt.Sprintf("cd %s && %s %s", shQuote(cwd), shQuote(claudeBin), strings.Join(args, " "))
		script := fmt.Sprintf("tell application \"Terminal\" to do script %s", asQuote(inner))
		_ = exec.Command("osascript", "-e", script, "-e", `tell application "Terminal" to activate`).Start()
		return nil
	}
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func asQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// openAgentPicker opens the picker to pin a worktree/agent to the focused
// quest. A no-op outside a quest detail view, or when no agents are running.
func (m *Model) openAgentPicker() {
	if m.modal == nil || m.modal.Kind != ModalQuestDetail {
		return
	}
	q := m.findQuest(m.modal.QuestID)
	if q == nil {
		return
	}
	// Fetch fresh so the picker is populated even before the sync loop has run
	// (it only fetches once a link exists). A brief blocking call on an explicit
	// keypress is fine.
	if agents, ok := fetchAgents(); ok {
		m.agents = agents
	}
	m.commitBodyLine()
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
