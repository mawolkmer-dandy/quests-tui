package app

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// herdr-native agent integration. `herdr workspace list` is the source of
// truth: each workspace carries a user-given label, a live agent_status
// (accurate — herdr reads the pane's terminal output, not Claude's daemon), and
// a stable workspace id. A quest pins workspaces by id; the status, name, and
// "open" all come from herdr. Requires the herdr server to be running — when
// it isn't, no agent state shows.

// HerdrWorkspace is one entry of `herdr workspace list`.
type HerdrWorkspace struct {
	ID     string // workspace_id, e.g. "wC"
	Label  string // user-given name, e.g. "S - questlog"
	Status string // "idle" | "working" | "blocked" | "unknown"
}

// workspacesMsg delivers a refreshed workspace list into Update.
type workspacesMsg struct{ ws []HerdrWorkspace }

// fetchHerdrWorkspaces runs `herdr workspace list` and parses it. ok is false
// when herdr isn't installed or its server isn't running.
func fetchHerdrWorkspaces() (ws []HerdrWorkspace, ok bool) {
	out, err := runCmd("herdr", "workspace", "list")
	if err != nil {
		return nil, false
	}
	var resp struct {
		Result struct {
			Workspaces []struct {
				WorkspaceID string `json:"workspace_id"`
				Label       string `json:"label"`
				AgentStatus string `json:"agent_status"`
			} `json:"workspaces"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, false
	}
	for _, w := range resp.Result.Workspaces {
		ws = append(ws, HerdrWorkspace{ID: w.WorkspaceID, Label: w.Label, Status: w.AgentStatus})
	}
	return ws, true
}

// refreshWorkspacesCmd fetches the workspace list off the UI goroutine.
func refreshWorkspacesCmd() tea.Cmd {
	return func() tea.Msg {
		ws, ok := fetchHerdrWorkspaces()
		if !ok {
			return nil
		}
		return workspacesMsg{ws: ws}
	}
}

// hasAgentLinks reports whether any quest pins a herdr workspace.
func (m *Model) hasAgentLinks() bool {
	for i := range m.store.Quests {
		if len(m.store.Quests[i].AgentWorkspaces) > 0 {
			return true
		}
	}
	return false
}

// workspace returns the cached herdr workspace with id, if present.
func (m *Model) workspace(id string) (HerdrWorkspace, bool) {
	for _, w := range m.workspaces {
		if w.ID == id {
			return w, true
		}
	}
	return HerdrWorkspace{}, false
}

// workspaceState is the display state for a pinned workspace: its herdr
// agent_status, or "none" when herdr doesn't know it (closed / server down).
func (m *Model) workspaceState(id string) string {
	if w, ok := m.workspace(id); ok {
		return w.Status
	}
	return "none"
}

// workspaceLabel is the workspace's herdr label, or its id when unknown.
func (m *Model) workspaceLabel(id string) string {
	if w, ok := m.workspace(id); ok && w.Label != "" {
		return w.Label
	}
	return id
}

// spinnerAgent is the braille dot-runner cycled for a working agent.
var spinnerAgent = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spin returns the current frame of set for the shared spinner clock.
func (m *Model) spin(set []string) string { return set[m.spinnerFrame%len(set)] }

// pulseStyle is the amber foreground for a fetching / CI-running icon at the
// current point in the pulse cycle (see ui.PulseAmber).
func (m *Model) pulseStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ui.PulseAmber[m.spinnerFrame%len(ui.PulseAmber)])
}

// agentGlyph is the icon for a herdr agent status: red for blocked, an animated
// braille spinner for working, green check for idle, muted otherwise.
func (m *Model) agentGlyph(status string) string {
	switch status {
	case "blocked":
		return lipgloss.NewStyle().Foreground(ui.ColorImportant).Render(ui.GlyphAgentBlocked)
	case "working":
		return ui.StyleRunning.Render(m.spin(spinnerAgent))
	case "idle":
		return lipgloss.NewStyle().Foreground(ui.ColorHeading).Render(ui.GlyphAgentIdle)
	default: // "unknown" / "none"
		return ui.StyleMuted.Render(ui.GlyphAgentNone)
	}
}

// agentWord is the status word shown beside the icon.
func agentWord(status string) string {
	switch status {
	case "none":
		return "no agent"
	case "":
		return "unknown"
	default:
		return status // idle / working / blocked / unknown
	}
}

// hasWorkingAgent reports whether any pinned workspace is working.
func (m *Model) hasWorkingAgent() bool {
	for i := range m.store.Quests {
		for _, id := range m.store.Quests[i].AgentWorkspaces {
			if m.workspaceState(id) == "working" {
				return true
			}
		}
	}
	return false
}

// hasAnimatedIntegration reports whether anything on screen needs the spinner
// clock: a working agent, a PR whose CI is running, or a linked code still
// awaiting its first sync ("fetching").
func (m *Model) hasAnimatedIntegration() bool {
	if !m.integrationsEnabled {
		return false
	}
	if m.hasWorkingAgent() {
		return true
	}
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		for _, c := range q.JiraCodes {
			if _, ok := m.jiraStatus[c]; !ok {
				return true // fetching
			}
		}
		for _, pr := range q.PRs {
			st, ok := m.prStatus[pr.Code]
			if !ok || st.Status == "running" {
				return true // fetching, or CI running
			}
		}
	}
	return false
}

// --- spinner clock (drives the agent braille + the fetching/CI pulse) --------

type spinnerTickMsg struct{ gen int }

func spinnerTick(gen int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinnerTickMsg{gen: gen} })
}

func (m *Model) maybeStartSpinner() tea.Cmd {
	if m.spinnerOn || !m.hasAnimatedIntegration() {
		return nil
	}
	m.spinnerOn = true
	m.spinnerGen++
	return spinnerTick(m.spinnerGen)
}

func (m *Model) onSpinnerTick(gen int) tea.Cmd {
	if gen != m.spinnerGen {
		return nil
	}
	if !m.hasAnimatedIntegration() {
		m.spinnerOn = false
		return nil
	}
	m.spinnerFrame++
	return spinnerTick(gen)
}

// --- herdr workspace poll (status refresh floor) -----------------------------

// workspacePollInterval is how often we re-query `herdr workspace list` while a
// workspace is pinned — short because it's a cheap local socket call and herdr
// state changes often.
const workspacePollInterval = 1500 * time.Millisecond

type agentPollTickMsg struct{ gen int }

func agentPollTick(gen int) tea.Cmd {
	return tea.Tick(workspacePollInterval, func(time.Time) tea.Msg { return agentPollTickMsg{gen: gen} })
}

// maybeStartAgentPoll starts the workspace poll if a workspace is pinned and it
// isn't already running.
func (m *Model) maybeStartAgentPoll() tea.Cmd {
	if m.agentPollOn || !m.integrationsEnabled || !m.hasAgentLinks() {
		return nil
	}
	m.agentPollOn = true
	m.agentPollGen++
	return tea.Batch(refreshWorkspacesCmd(), agentPollTick(m.agentPollGen))
}

// onAgentPollTick refreshes and re-arms while any workspace stays pinned.
func (m *Model) onAgentPollTick(gen int) tea.Cmd {
	if gen != m.agentPollGen {
		return nil
	}
	if !m.hasAgentLinks() {
		m.agentPollOn = false
		return nil
	}
	return tea.Batch(refreshWorkspacesCmd(), agentPollTick(gen))
}

// openWorkspace focuses a herdr workspace (jumps to its pane), fire-and-forget.
func openWorkspace(id string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command("herdr", "workspace", "focus", id).Start()
		return nil
	}
}

// --- picker ------------------------------------------------------------------

// openAgentPicker opens the picker to pin a herdr workspace to the focused
// quest. A no-op outside a quest detail view.
func (m *Model) openAgentPicker() {
	if m.modal == nil || m.modal.Kind != ModalQuestDetail {
		return
	}
	q := m.findQuest(m.modal.QuestID)
	if q == nil {
		return
	}
	// Fetch fresh so the picker reflects herdr right now.
	if ws, ok := fetchHerdrWorkspaces(); ok {
		m.workspaces = ws
	}
	m.commitBodyLine()
	m.clearFocusLink()
	m.pushModal(&Modal{Kind: ModalAgentPicker, TargetQuestID: q.ID, PickerItems: m.workspacePickerItems()})
}

// workspacePickerItems lists herdr workspaces for the picker, labelled
// "<name> · <status>"; the ID is the workspace id that gets pinned.
func (m *Model) workspacePickerItems() []pickerItem {
	var items []pickerItem
	for _, w := range m.workspaces {
		label := w.Label
		if label == "" {
			label = w.ID
		}
		items = append(items, pickerItem{ID: w.ID, Label: fmt.Sprintf("%s · %s", label, agentWord(w.Status))})
	}
	return items
}
