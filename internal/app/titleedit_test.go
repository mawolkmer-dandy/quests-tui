package app

import (
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func typeText(m *Model, s string) {
	for _, r := range s {
		m.updateModal(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func TestTitleEditQuest(t *testing.T) {
	q := model.Quest{ID: "q1", Title: "Old title"}
	st := &store.Store{Quests: []model.Quest{q}}
	m := &Model{store: st, modal: &Modal{Kind: ModalQuestDetail, QuestID: "q1"}}

	m.beginTitleEdit()
	if m.titleEditor == nil {
		t.Fatal("beginTitleEdit did not open an editor")
	}
	if got := m.titleEditor.Value(); got != "Old title" {
		t.Fatalf("editor seeded with %q, want %q", got, "Old title")
	}
	// Select-all-ish: clear then type. Simplest: append text and commit.
	typeText(m, " EDITED")
	m.commitTitleEdit()

	if m.titleEditor != nil {
		t.Fatal("commit did not close the editor")
	}
	if got := m.findQuest("q1").Title; got != "Old title EDITED" {
		t.Fatalf("title = %q, want %q", got, "Old title EDITED")
	}
}

func TestTitleEditCancelKeepsOld(t *testing.T) {
	st := &store.Store{Quests: []model.Quest{{ID: "q1", Title: "Keep me"}}}
	m := &Model{store: st, modal: &Modal{Kind: ModalQuestDetail, QuestID: "q1"}}
	m.beginTitleEdit()
	typeText(m, " junk")
	m.cancelTitleEdit()
	if got := m.findQuest("q1").Title; got != "Keep me" {
		t.Fatalf("title = %q, want unchanged %q", got, "Keep me")
	}
}

func TestTitleEditBlankIgnored(t *testing.T) {
	st := &store.Store{Quests: []model.Quest{{ID: "q1", Title: "Not blank"}}}
	m := &Model{store: st, modal: &Modal{Kind: ModalQuestDetail, QuestID: "q1"}}
	m.beginTitleEdit()
	m.titleEditor.SetValue("   ")
	m.commitTitleEdit()
	if got := m.findQuest("q1").Title; got != "Not blank" {
		t.Fatalf("blank title accepted: %q", got)
	}
}

func TestUpAtBodyTopEntersTitleEdit(t *testing.T) {
	st := &store.Store{Quests: []model.Quest{{ID: "q1", Title: "T", Body: []model.BodyLine{{Text: "only line"}}}}}
	m := &Model{store: st, modal: &Modal{Kind: ModalQuestDetail, QuestID: "q1"}}
	mod := m.modal
	mod.BodyCursor = 0
	mod.BodyEditor = m.newBodyEditor("only line")
	mod.BodyEditor.SetCursor(0)

	m.updateModal(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.titleEditor == nil {
		t.Fatal("Up at body top did not enter title edit")
	}
}

func TestDownFromTitleReturnsToBody(t *testing.T) {
	st := &store.Store{Quests: []model.Quest{{ID: "q1", Title: "T", Body: []model.BodyLine{{Text: "line a"}, {Text: "line b"}}}}}
	m := &Model{store: st, modal: &Modal{Kind: ModalQuestDetail, QuestID: "q1"}}
	mod := m.modal
	mod.BodyCursor = 0
	mod.BodyEditor = m.newBodyEditor("line a")
	mod.BodyEditor.SetCursor(0)

	m.updateModal(tea.KeyPressMsg{Code: tea.KeyUp}) // into title
	if m.titleEditor == nil {
		t.Fatal("Up did not enter title edit")
	}
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyDown}) // back to body top
	if m.titleEditor != nil {
		t.Fatal("Down did not leave title edit")
	}
	if mod.BodyCursor != 0 {
		t.Fatalf("body cursor = %d, want 0 (top)", mod.BodyCursor)
	}
	if m.onFocusLink() {
		t.Fatal("returned to Sigils, want body")
	}
}

func TestDownFromTitleReturnsToSigils(t *testing.T) {
	m := &Model{integrationsEnabled: true, titleEditFromSigils: true, focusLinkIdx: 3}
	m.returnFromTitleEdit()
	if m.focusLinkIdx != 0 {
		t.Fatalf("focusLinkIdx = %d, want 0 (top of Sigils)", m.focusLinkIdx)
	}
}

func TestBodyCaretActivePredicate(t *testing.T) {
	base := func() *Model {
		return &Model{integrationsEnabled: true, focusLinkIdx: noSelection, modal: &Modal{Kind: ModalQuestDetail}}
	}
	if !base().bodyCaretActive() {
		t.Fatal("plain body should own the caret")
	}
	m := base()
	m.titleEditor = &textinput.Model{}
	if m.bodyCaretActive() {
		t.Fatal("title editor active: body must NOT draw a caret")
	}
	m = base()
	m.focusLinkIdx = 0 // on a Sigils link
	if m.bodyCaretActive() {
		t.Fatal("Sigils focused: body must NOT draw a caret")
	}
	m = base()
	m.modal.InQuestList = true
	if m.bodyCaretActive() {
		t.Fatal("quest list focused: body must NOT draw a caret")
	}
}

func TestTitleEditCampaign(t *testing.T) {
	st := &store.Store{Projects: []model.Project{{ID: "p1", Name: "Old camp"}}}
	m := &Model{store: st, modal: &Modal{Kind: ModalCampaignDetail, CampaignID: "p1"}}
	m.beginTitleEdit()
	typeText(m, "X")
	m.commitTitleEdit()
	if got := m.findProject("p1").Name; got != "Old campX" {
		t.Fatalf("campaign name = %q", got)
	}
}
