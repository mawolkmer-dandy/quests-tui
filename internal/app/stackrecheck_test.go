package app

import (
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
)

func recheckModel(quests []model.Quest, sts ...PRStatus) *Model {
	m := &Model{
		store:               &store.Store{Quests: quests},
		prStatus:            map[string]PRStatus{},
		integrationsEnabled: true,
	}
	for _, st := range sts {
		m.prStatus[st.Code] = st
	}
	return m
}

func TestStackExpandSeed(t *testing.T) {
	t.Run("prefers the first open member over a merged one", func(t *testing.T) {
		m := recheckModel(nil,
			PRStatus{Code: "#1", Status: "merged"},
			PRStatus{Code: "#2", Status: "success"},
			PRStatus{Code: "#3", Status: "running"},
		)
		q := &model.Quest{PRs: []model.PRLink{{Code: "#1"}, {Code: "#2"}, {Code: "#3"}}}
		seed, ok := m.stackExpandSeed(q)
		if !ok || seed.Code != "#2" {
			t.Fatalf("got seed=%q ok=%v, want #2/true", seed.Code, ok)
		}
	})

	t.Run("falls back to an unfetched member when none is known-open", func(t *testing.T) {
		m := recheckModel(nil, PRStatus{Code: "#1", Status: "merged"})
		q := &model.Quest{PRs: []model.PRLink{{Code: "#1"}, {Code: "#2"}}} // #2 has no status yet
		seed, ok := m.stackExpandSeed(q)
		if !ok || seed.Code != "#2" {
			t.Fatalf("got seed=%q ok=%v, want #2/true", seed.Code, ok)
		}
	})

	t.Run("skips a fully settled stack", func(t *testing.T) {
		m := recheckModel(nil,
			PRStatus{Code: "#1", Status: "merged"},
			PRStatus{Code: "#2", Status: "closed"},
		)
		q := &model.Quest{PRs: []model.PRLink{{Code: "#1"}, {Code: "#2"}}}
		if _, ok := m.stackExpandSeed(q); ok {
			t.Fatal("a merged+closed stack must not produce a seed")
		}
	})
}

func TestAutoStackExpandCmdGating(t *testing.T) {
	activeQuest := model.Quest{ID: "q1", PRs: []model.PRLink{{Code: "#1", Repo: "o/r"}}}

	t.Run("nil when integrations are off", func(t *testing.T) {
		m := recheckModel([]model.Quest{activeQuest}, PRStatus{Code: "#1", Status: "running"})
		m.integrationsEnabled = false
		if cmd := m.autoStackExpandCmd(); cmd != nil {
			t.Fatal("must not fire when integrations are disabled")
		}
	})

	t.Run("nil when every quest is settled, vaulted, or PR-less", func(t *testing.T) {
		m := recheckModel([]model.Quest{
			{ID: "settled", PRs: []model.PRLink{{Code: "#1"}}},
			{ID: "vaulted", Vaulted: true, PRs: []model.PRLink{{Code: "#2"}}},
			{ID: "no-prs"},
		},
			PRStatus{Code: "#1", Status: "merged"},
			PRStatus{Code: "#2", Status: "running"},
		)
		if cmd := m.autoStackExpandCmd(); cmd != nil {
			t.Fatal("no active quest — must return nil")
		}
	})

	t.Run("fires for a quest with an open PR", func(t *testing.T) {
		m := recheckModel([]model.Quest{activeQuest}, PRStatus{Code: "#1", Status: "running"})
		if cmd := m.autoStackExpandCmd(); cmd == nil {
			t.Fatal("an active-stack quest must produce a re-check command")
		}
	})
}
