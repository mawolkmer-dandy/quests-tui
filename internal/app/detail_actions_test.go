package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func ctrlKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

func newDetailModel() *Model {
	m := &Model{
		store: &store.Store{
			Projects: []model.Project{{ID: "p1", Name: "P"}},
			Quests:   []model.Quest{{ID: "q1", Title: "Q", ProjectID: "p1"}},
		},
		modal:             &Modal{Kind: ModalQuestDetail, QuestID: "q1"},
		width:             120,
		height:            40,
		collapsedProjects: map[string]bool{},
	}
	m.seedBody(ownerQuest, "q1")
	return m
}

// The always-safe quest actions (priority, type, move, done) work from the body.
func TestQuestActionsFromBody(t *testing.T) {
	ui.Init(true)

	m := newDetailModel()
	m.clearFocusLink() // caret in the body
	m.updateModal(ctrlKey('p'))
	if m.store.Quests[0].Priority != model.PriorityMedium {
		t.Errorf("Ctrl+P should cycle priority from the body; got %q", m.store.Quests[0].Priority)
	}

	m = newDetailModel()
	m.clearFocusLink()
	m.updateModal(ctrlKey('t'))
	if m.store.Quests[0].Type != model.QuestTypeMain {
		t.Errorf("Ctrl+T should flip type from the body; got %q", m.store.Quests[0].Type)
	}

	m = newDetailModel()
	m.clearFocusLink()
	m.updateModal(ctrlKey('d'))
	if m.store.Quests[0].Status != model.StatusDone {
		t.Errorf("Ctrl+D on a plain body should mark done; got %q", m.store.Quests[0].Status)
	}
}

// Active / schedule / vault act on the quest only from the Sigils pane — in the
// body they belong to the text editor (Cmd+←/→ = line start/end, Ctrl+V paste).
func TestEditorKeysDeferToBody(t *testing.T) {
	ui.Init(true)

	// In the body, Ctrl+E must NOT open the schedule picker (it's line-end).
	m := newDetailModel()
	m.clearFocusLink()
	m.updateModal(ctrlKey('e'))
	if m.modal.Kind == ModalSchedulePicker {
		t.Error("Ctrl+E in the body should be line-end, not open the schedule picker")
	}

	// In the body, Ctrl+A must NOT take the quest up (it's line-start).
	m = newDetailModel()
	m.clearFocusLink()
	m.updateModal(ctrlKey('a'))
	if m.store.Quests[0].Status == model.StatusActive {
		t.Error("Ctrl+A in the body should be line-start, not toggle active")
	}

	// With a sigil focused, the same keys act on the quest.
	m = newDetailModel()
	m.focusLinkIdx = 0 // a sigil is focused
	m.updateModal(ctrlKey('a'))
	if m.store.Quests[0].Status != model.StatusActive {
		t.Errorf("Ctrl+A from the Sigils pane should take the quest up; got %q", m.store.Quests[0].Status)
	}

	m = newDetailModel()
	m.focusLinkIdx = 0
	m.updateModal(ctrlKey('v'))
	if !m.store.Quests[0].Vaulted {
		t.Error("Ctrl+V from the Sigils pane should vault the quest")
	}

	m = newDetailModel()
	m.focusLinkIdx = 0
	m.updateModal(ctrlKey('e'))
	if m.modal.Kind != ModalSchedulePicker {
		t.Errorf("Ctrl+E from the Sigils pane should open the schedule picker; got %v", m.modal.Kind)
	}
}

// Ctrl+D on an objective body line checks the objective off — it must NOT mark
// the whole quest done.
func TestCtrlDChecksObjectiveNotQuest(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store: &store.Store{
			Projects: []model.Project{{ID: "p1", Name: "P"}},
			Quests: []model.Quest{{
				ID: "q1", Title: "Q", ProjectID: "p1",
				Body: []model.BodyLine{{ID: "b1", Text: "- obj"}},
			}},
		},
		modal:             &Modal{Kind: ModalQuestDetail, QuestID: "q1"},
		width:             120,
		height:            40,
		collapsedProjects: map[string]bool{},
	}
	m.seedBody(ownerQuest, "q1") // caret on the "- obj" line
	m.clearFocusLink()           // in the body, not on a sigil

	m.updateModal(ctrlKey('d'))

	q := &m.store.Quests[0]
	if q.Status == model.StatusDone {
		t.Error("Ctrl+D on an objective line must not mark the quest done")
	}
	if !q.Body[0].Done {
		t.Error("Ctrl+D on an objective line should check the objective off")
	}
}
