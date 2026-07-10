package app

import (
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// agentsDirtyMsg is delivered when a Claude session file changes — i.e. an
// agent's status transitioned — a cue to re-fetch `claude agents --json`.
type agentsDirtyMsg struct{}

// claudeSessionsDir is where Claude Code writes one JSON file per live session,
// rewritten on each status change (see agents.go). Honors CLAUDE_CONFIG_DIR,
// else ~/.claude.
func claudeSessionsDir() string {
	base := os.Getenv("CLAUDE_CONFIG_DIR")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".claude")
	}
	return filepath.Join(base, "sessions")
}

// watchAgentSessions watches Claude's sessions directory so agent-state changes
// refresh near-instantly instead of waiting for the sync tick. Those files are
// rewritten only on an actual status transition (not as a heartbeat), so this
// is event-driven and idle-free — far cheaper than polling `claude agents`
// every second. Returns nil if the watcher can't be set up (the periodic sync
// still refreshes agents as a fallback).
func (m *Model) watchAgentSessions() tea.Cmd {
	dir := claudeSessionsDir()
	if dir == "" {
		return nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil
	}
	m.agentWatcher = w
	return waitForAgentEvent(w)
}

// waitForAgentEvent blocks until a session file changes, coalesces the burst a
// single write produces into one signal, and reports agentsDirtyMsg. Re-issued
// after each refresh to keep listening.
func waitForAgentEvent(w *fsnotify.Watcher) tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return nil
				}
				if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 || filepath.Ext(ev.Name) != ".json" {
					continue
				}
				// Coalesce the rest of the burst (repeated writes/chmods) so a
				// status change triggers a single re-fetch.
				deadline := time.After(150 * time.Millisecond)
				for draining := true; draining; {
					select {
					case _, ok := <-w.Events:
						if !ok {
							return agentsDirtyMsg{}
						}
					case <-deadline:
						draining = false
					}
				}
				return agentsDirtyMsg{}
			case _, ok := <-w.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
}
