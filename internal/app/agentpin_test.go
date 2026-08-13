package app

import (
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
)

func agentModel() *Model {
	return &Model{
		agents: []HerdrAgent{
			{ID: "term_aaa", Session: "sess-uuid-1", WorkspaceID: "w15", Workspace: "main", Tab: "task a", Status: "working"},
			{ID: "term_bbb", Session: "sess-uuid-2", WorkspaceID: "w15", Workspace: "main", Tab: "task b", Status: "idle"},
			{ID: "term_ccc", Session: "sess-uuid-3", WorkspaceID: "w0", Workspace: "aisr left sidebar", Tab: "1", Status: "blocked"},
		},
	}
}

func TestMatchAgentBySession(t *testing.T) {
	m := agentModel()
	a, ok := m.matchAgent("sess-uuid-2")
	if !ok || a.ID != "term_bbb" {
		t.Fatalf("session pin didn't resolve: %+v ok=%v", a, ok)
	}
}

func TestMatchAgentByTerminal(t *testing.T) {
	m := agentModel()
	a, ok := m.matchAgent("term_ccc")
	if !ok || a.Status != "blocked" {
		t.Fatalf("terminal pin didn't resolve: %+v ok=%v", a, ok)
	}
}

func TestMatchAgentByLegacyWorkspace(t *testing.T) {
	m := agentModel()
	// The "w0 / no agent" bug: a legacy workspace-id pin must resolve to a real
	// agent (its first in that workspace), not fall through to the raw id.
	a, ok := m.matchAgent("w0")
	if !ok {
		t.Fatal("legacy workspace pin 'w0' did not resolve")
	}
	if a.Workspace != "aisr left sidebar" {
		t.Fatalf("resolved wrong workspace: %q", a.Workspace)
	}
	if m.agentLabel("w0") == "w0" {
		t.Fatal("agentLabel still shows raw 'w0'")
	}
	if m.agentState("w0") != "blocked" {
		t.Fatalf("agentState(w0) = %q, want blocked", m.agentState("w0"))
	}
}

func TestMatchAgentUnknownPin(t *testing.T) {
	m := agentModel()
	if _, ok := m.matchAgent("term_gone"); ok {
		t.Fatal("a stale pin should NOT resolve")
	}
	if m.agentState("term_gone") != "none" {
		t.Fatal("stale pin should read as none/no agent")
	}
}

func TestHealMigratesTerminalToSession(t *testing.T) {
	m := agentModel()
	m.store = &store.Store{Quests: []model.Quest{
		{ID: "q1", AgentWorkspaces: []string{"term_aaa"}},    // ephemeral terminal pin
		{ID: "q2", AgentWorkspaces: []string{"w0"}},          // ambiguous legacy workspace pin
		{ID: "q3", AgentWorkspaces: []string{"sess-uuid-3"}}, // already stable
	}}
	if !m.healAgentPins() {
		t.Fatal("heal should have migrated the terminal pin")
	}
	if got := m.store.Quests[0].AgentWorkspaces[0]; got != "sess-uuid-1" {
		t.Fatalf("terminal pin not migrated to session: %q", got)
	}
	if got := m.store.Quests[1].AgentWorkspaces[0]; got != "w0" {
		t.Fatalf("ambiguous workspace pin must be left alone, got %q", got)
	}
	if got := m.store.Quests[2].AgentWorkspaces[0]; got != "sess-uuid-3" {
		t.Fatalf("stable pin should be untouched, got %q", got)
	}
	if m.healAgentPins() {
		t.Fatal("second heal should be a no-op (idempotent)")
	}
}
