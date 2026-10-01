package app

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func delModel(t *testing.T) *Model {
	ui.Init(true)
	return &Model{
		store: &store.Store{
			Banners: []model.Banner{{ID: "b1", Name: "Platform"}},
			Projects: []model.Project{
				{ID: "c1", Name: "Migrate", BannerID: "b1"},
				{ID: "c2", Name: "Solo"},
			},
			Quests: []model.Quest{
				{ID: "q1", Title: "Fix", ProjectID: "c1", BannerID: "b1"},
				{ID: "q2", Title: "Add", ProjectID: "c1", BannerID: "b1"},
				{ID: "lq1", Title: "loose", BannerID: "b1"},
			},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{},
		sectionScroll:     map[string]int{},
		sectionMaxScroll:  map[string]int{},
	}
}

// The one dialog carries entity-specific copy and live counts, and always opens
// with Cancel focused so deleting takes a deliberate move.
func TestDeleteModalCopyPerEntity(t *testing.T) {
	cases := []struct {
		name      string
		target    cursorTarget
		wantTitle string
		wantBody  string
		wantVerb  string
	}{
		{"banner", cursorTarget{kind: ui.RowBanner, bannerID: "b1"},
			`Delete banner "Platform"?`, "Its 2 items will move to Unassigned — not deleted.", "Delete"}, // c1 + lq1
		{"campaign", cursorTarget{kind: ui.RowProject, projectID: "c1"},
			`Delete campaign "Migrate"?`, "It contains 2 quests that will be deleted too.", "Delete"},
		{"empty-campaign", cursorTarget{kind: ui.RowProject, projectID: "c2"},
			`Delete campaign "Solo"?`, "", "Delete"},
		{"quest", cursorTarget{kind: ui.RowQuest, questID: "q1"},
			`Delete quest "Fix"?`, "", "Delete"},
		{"rune", cursorTarget{kind: ui.RowRune, runeKey: "flag"},
			`Stop watching rune "flag"?`, "", "Stop watching"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := delModel(t)
			if !m.openDeleteModal(tc.target) {
				t.Fatalf("openDeleteModal(%s) returned false", tc.name)
			}
			mod := m.modal
			if mod.Kind != ModalConfirmDelete {
				t.Fatalf("wrong modal kind %v", mod.Kind)
			}
			if mod.Title != tc.wantTitle || mod.Body != tc.wantBody || mod.DeleteVerb != tc.wantVerb {
				t.Fatalf("copy mismatch:\n got  title=%q body=%q verb=%q\n want title=%q body=%q verb=%q",
					mod.Title, mod.Body, mod.DeleteVerb, tc.wantTitle, tc.wantBody, tc.wantVerb)
			}
			if mod.DeleteFocus != 1 {
				t.Fatalf("dialog should open focused on the delete button, got focus %d", mod.DeleteFocus)
			}
		})
	}
}

// A non-deletable target opens no dialog.
func TestDeleteModalRejectsNonDeletable(t *testing.T) {
	m := delModel(t)
	if m.openDeleteModal(cursorTarget{kind: ui.RowSection, section: "inbox"}) {
		t.Fatal("a section is not deletable — no dialog should open")
	}
	if m.modal != nil {
		t.Fatal("no modal should have been pushed")
	}
}

// Confirming a campaign deletion removes it and its quests and closes the
// dialog; Esc cancels with no change.
func TestDeleteModalConfirmAndCancel(t *testing.T) {
	// Cancel: Esc leaves everything.
	m := delModel(t)
	m.cursor = cursorTarget{kind: ui.RowProject, projectID: "c1"}
	m.openDeleteModal(m.cursor)
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.modal != nil {
		t.Fatal("Esc should close the dialog")
	}
	if len(m.store.Projects) != 2 || len(m.store.Quests) != 3 {
		t.Fatalf("cancel must change nothing: %d projects, %d quests", len(m.store.Projects), len(m.store.Quests))
	}

	// Confirm: the campaign and its two quests go; the loose quest + other
	// campaign stay.
	m = delModel(t)
	m.cursor = cursorTarget{kind: ui.RowProject, projectID: "c1"}
	m.openDeleteModal(m.cursor)
	m.updateModal(tea.KeyPressMsg{Text: "d"}) // 'd' confirms
	if m.modal != nil {
		t.Fatal("confirming should close the dialog")
	}
	if m.findProject("c1") != nil {
		t.Fatal("campaign c1 should be deleted")
	}
	if len(m.store.Quests) != 1 || m.store.Quests[0].ID != "lq1" {
		t.Fatalf("only the loose quest should remain, got %+v", m.store.Quests)
	}
	if m.findProject("c2") == nil {
		t.Fatal("the unrelated campaign must survive")
	}
}
