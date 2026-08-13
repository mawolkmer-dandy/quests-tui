package app

import (
	"encoding/json"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// herdr-native agent integration. `herdr agent list` is the source of truth:
// each entry is one live Claude agent pane, carrying its terminal title, a live
// agent_status (accurate — herdr reads the pane's terminal output, not Claude's
// daemon), and a stable terminal id. A quest pins agents by that terminal id;
// the status, name, and "open" all come from herdr. Requires the herdr server
// to be running — when it isn't, no agent state shows.

// HerdrAgent is one entry of `herdr agent list` — a live agent pane, named the
// way herdr's own sidebar names it: "<workspace> · <tab>".
type HerdrAgent struct {
	ID          string // terminal_id, e.g. "term_6579824d674f05" — the focus target, but EPHEMERAL (rotates when a pane is closed/recreated or herdr restarts)
	Session     string // agent_session.value — the Claude conversation UUID; the most STABLE identity (survives terminal rotation)
	WorkspaceID string // workspace_id, e.g. "w0" / "w15" — what an older version of this app pinned
	Workspace   string // herdr workspace label, e.g. "main" / "impressions-smokescreen"
	Tab         string // herdr tab label, e.g. "better checkout" (often "1" for single-tab workspaces)
	Status      string // "idle" | "working" | "blocked" | "done" | "unknown"
}

// pinKey is the identity persisted when a quest pins this agent: the Claude
// session UUID when known (stable across terminal rotation), else the terminal
// id. matchAgent resolves either back to the live agent.
func (a HerdrAgent) pinKey() string {
	if a.Session != "" {
		return a.Session
	}
	return a.ID
}

// Name is the agent's herdr-style display name, "<workspace> · <tab>".
func (a HerdrAgent) Name() string {
	if a.Workspace == "" {
		return a.Tab
	}
	if a.Tab == "" {
		return a.Workspace
	}
	return a.Workspace + " · " + a.Tab
}

// agentsMsg delivers a refreshed agent list into Update.
type agentsMsg struct{ agents []HerdrAgent }

// herdrLabels runs a `herdr <thing> list` command and returns an id→label map,
// keyed on the given id field ("workspace_id" or "tab_id"). Best-effort: an
// error yields an empty map, so the agent still shows with whatever it has.
func herdrLabels(thing, listKey, idKey string) map[string]string {
	out, err := runCmd("herdr", thing, "list")
	if err != nil {
		return map[string]string{}
	}
	// result mixes the entry array with scalar fields (e.g. "type"), so decode
	// it loosely and pull just the list we want.
	var resp struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return map[string]string{}
	}
	var raw []map[string]any
	if err := json.Unmarshal(resp.Result[listKey], &raw); err != nil {
		return map[string]string{}
	}
	labels := map[string]string{}
	for _, e := range raw {
		id, _ := e[idKey].(string)
		label, _ := e["label"].(string)
		if id != "" {
			labels[id] = label
		}
	}
	return labels
}

// fetchHerdrAgents runs `herdr agent list` and joins each agent to its herdr
// workspace and tab labels (from `herdr workspace list` / `herdr tab list`), so
// agents read the way they do in herdr's sidebar. ok is false when herdr isn't
// installed or its server isn't running. Bare-shell panes (no agent) are
// skipped — only actual agents are pinnable.
func fetchHerdrAgents() (agents []HerdrAgent, ok bool) {
	out, err := runCmd("herdr", "agent", "list")
	if err != nil {
		return nil, false
	}
	var resp struct {
		Result struct {
			Agents []struct {
				Agent        string `json:"agent"`
				AgentStatus  string `json:"agent_status"`
				TerminalID   string `json:"terminal_id"`
				TabID        string `json:"tab_id"`
				WorkspaceID  string `json:"workspace_id"`
				AgentSession struct {
					Value string `json:"value"`
				} `json:"agent_session"`
			} `json:"agents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, false
	}
	workspaces := herdrLabels("workspace", "workspaces", "workspace_id")
	tabs := herdrLabels("tab", "tabs", "tab_id")
	for _, a := range resp.Result.Agents {
		if a.Agent == "" || a.TerminalID == "" {
			continue // a bare-shell pane, not an agent
		}
		agents = append(agents, HerdrAgent{
			ID:          a.TerminalID,
			Session:     a.AgentSession.Value,
			WorkspaceID: a.WorkspaceID,
			Workspace:   workspaces[a.WorkspaceID],
			Tab:         tabs[a.TabID],
			Status:      a.AgentStatus,
		})
	}
	return agents, true
}

// healAgentPins migrates persisted pins to the stable session UUID whenever a
// fresh agent list lets us resolve them — so a pin stored as an ephemeral
// terminal id or a legacy workspace id upgrades itself the next time herdr
// reports the agent, instead of decaying into "no agent" later. Returns true
// if anything changed (so the caller can persist).
func (m *Model) healAgentPins() bool {
	changed := false
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		for j, pin := range q.AgentWorkspaces {
			a, ok := m.matchAgent(pin)
			if !ok {
				continue
			}
			// Only auto-migrate UNAMBIGUOUS matches (session/terminal → session).
			// A legacy workspace-id pin maps to several agents, so leave it in
			// place — matchAgent still resolves it for display, and silently
			// committing it to one agent could pick the wrong one.
			if pin != a.Session && pin != a.ID {
				continue
			}
			if stable := a.pinKey(); stable != "" && stable != pin {
				q.AgentWorkspaces[j] = stable
				changed = true
			}
		}
	}
	return changed
}

// refreshAgentsCmd fetches the agent list off the UI goroutine.
func refreshAgentsCmd() tea.Cmd {
	return func() tea.Msg {
		agents, ok := fetchHerdrAgents()
		if !ok {
			return nil
		}
		return agentsMsg{agents: agents}
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

// matchAgent resolves a persisted pin to a live herdr agent. A pin may be a
// session UUID (current scheme), a terminal id (older), or a workspace id (the
// oldest — what "AgentWorkspaces" was named after). Session/terminal are exact
// and specific; workspace is ambiguous (a workspace can hold several agents),
// so it's only used as a fallback, resolving to that workspace's first agent —
// enough to show a real name/status instead of the raw "w0 / no agent".
func (m *Model) matchAgent(pin string) (HerdrAgent, bool) {
	if pin == "" {
		return HerdrAgent{}, false
	}
	for _, a := range m.agents {
		if pin == a.Session || pin == a.ID {
			return a, true
		}
	}
	for _, a := range m.agents {
		if pin == a.WorkspaceID {
			return a, true
		}
	}
	return HerdrAgent{}, false
}

// agentState is the display state for a pinned agent: its herdr agent_status,
// or "none" when herdr doesn't know it (closed / server down).
func (m *Model) agentState(id string) string {
	if a, ok := m.matchAgent(id); ok {
		return a.Status
	}
	return "none"
}

// agentLabel is the agent's herdr-style name ("<workspace> · <tab>"), or its
// id when herdr doesn't know it (closed / server down).
func (m *Model) agentLabel(id string) string {
	if a, ok := m.matchAgent(id); ok && a.Name() != "" {
		return a.Name()
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
	case "idle", "done":
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
		return status // idle / working / blocked / done / unknown
	}
}

// hasWorkingAgent reports whether any pinned workspace is working.
func (m *Model) hasWorkingAgent() bool {
	for i := range m.store.Quests {
		for _, id := range m.store.Quests[i].AgentWorkspaces {
			if m.agentState(id) == "working" {
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
	return tea.Batch(refreshAgentsCmd(), agentPollTick(m.agentPollGen))
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
	return tea.Batch(refreshAgentsCmd(), agentPollTick(gen))
}

// openAgent focuses a pinned herdr agent (jumps to its pane), fire-and-forget.
// The pin is resolved to the agent's CURRENT terminal id first — herdr's focus
// target must be a live terminal/name/pane, and the pinned key may be a session
// or (legacy) workspace id that isn't a valid target on its own.
func (m *Model) openAgent(pin string) tea.Cmd {
	target := pin
	if a, ok := m.matchAgent(pin); ok && a.ID != "" {
		target = a.ID
	}
	return func() tea.Msg {
		_ = exec.Command("herdr", "agent", "focus", target).Start()
		return nil
	}
}

// --- picker ------------------------------------------------------------------

// openAgentPicker opens the picker to pin a herdr agent to the focused quest.
// A no-op outside a quest detail view.
func (m *Model) openAgentPicker() tea.Cmd {
	if m.modal == nil || m.modal.Kind != ModalQuestDetail {
		return nil
	}
	q := m.findQuest(m.modal.QuestID)
	if q == nil {
		return nil
	}
	// Fetch fresh so the picker reflects herdr right now.
	if agents, ok := fetchHerdrAgents(); ok {
		m.agents = agents
	}
	m.commitBodyLine()
	m.clearFocusLink()
	m.pushModal(&Modal{Kind: ModalAgentPicker, TargetQuestID: q.ID, PickerItems: m.agentPickerItems()})
	return m.playSound(sndOpenNPC)
}

// agentPickerItems lists herdr agents for the picker. Label is the plain
// "<workspace> · <tab>" name (used for fuzzy filtering); the row is rendered
// with a status icon + styling in renderModal. ID is the pinKey persisted on
// the quest (session UUID when known) so the pin survives terminal rotation.
func (m *Model) agentPickerItems() []pickerItem {
	var items []pickerItem
	for _, a := range m.agents {
		label := a.Name()
		if label == "" {
			label = a.pinKey()
		}
		items = append(items, pickerItem{ID: a.pinKey(), Label: label})
	}
	return items
}
