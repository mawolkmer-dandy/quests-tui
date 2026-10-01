package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Ctrl+E inside a quest's detail page opens the schedule picker for that quest —
// not only from a list.
func TestScheduleFromQuestDetail(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store:  &store.Store{Quests: []model.Quest{{ID: "q1", Title: "Sched me"}}},
		modal:  &Modal{Kind: ModalQuestDetail, QuestID: "q1"},
		width:  120,
		height: 40,
	}

	m.updateModal(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})

	if m.modal == nil || m.modal.Kind != ModalSchedulePicker || m.modal.TargetQuestID != "q1" {
		t.Fatalf("Ctrl+E in the quest detail should open the schedule picker for q1; modal=%+v", m.modal)
	}
	// The detail modal is preserved beneath, so dismissing the picker returns to it.
	if len(m.modalStack) == 0 || m.modalStack[len(m.modalStack)-1].Kind != ModalQuestDetail {
		t.Error("the quest detail should remain on the stack beneath the picker")
	}
}

// The detail's Sigils metadata shows a Schedule row only when the quest has a
// muster or rite — hidden otherwise.
func TestScheduleMetaRowVisibility(t *testing.T) {
	ui.Init(true)
	m := &Model{store: &store.Store{}}
	q := &model.Quest{ID: "q", Title: "Q"}

	if got := strings.Join(m.focusCodeLines(q, 0, 0), "\n"); strings.Contains(got, "Schedule") {
		t.Error("an unscheduled quest should not show a Schedule metadata row")
	}

	q.Recurrence = &model.Recurrence{Every: model.RecurDaily}
	got := strings.Join(m.focusCodeLines(q, 0, 0), "\n")
	if !strings.Contains(got, "Schedule") || !strings.Contains(got, "daily") {
		t.Errorf("a rite quest should show a 'Schedule … daily' row; got:\n%s", got)
	}
}
