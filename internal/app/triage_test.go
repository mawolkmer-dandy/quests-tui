package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// triageModel: two campaigns (p1 under banner b1 with a recent active quest, p2
// ungrouped and untouched) plus two untriaged Questboard quests.
func triageModel(t *testing.T) *Model {
	ui.Init(true)
	old := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)
	m := &Model{
		store: &store.Store{
			Banners:  []model.Banner{{ID: "b1", Name: "Platform"}},
			Projects: []model.Project{{ID: "p1", Name: "Migrate", BannerID: "b1"}, {ID: "p2", Name: "Ops"}},
			Quests: []model.Quest{
				{ID: "pa", Title: "existing", ProjectID: "p1", Status: model.StatusActive, UpdatedAt: recent},
				{ID: "qb1", Title: "Triage me", UpdatedAt: old},
				{ID: "qb2", Title: "Also triage", UpdatedAt: old},
			},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{},
		sectionScroll:     map[string]int{},
		sectionMaxScroll:  map[string]int{},
	}
	m.width, m.height = 100, 30
	m.lastSnapshot = m.store.Snapshot()
	return m
}

// pickerIndexFor finds a campaign's row in the (recents-ordered) picker list.
func pickerIndexFor(mod *Modal, id string) int {
	for i, it := range mod.PickerItems {
		if it.ID == id {
			return i
		}
	}
	return -1
}

// Ctrl+O files a Questboard quest into a campaign as OPEN, inheriting the
// campaign's banner and leaving its status untouched.
func TestTriageFileOpenInheritsBanner(t *testing.T) {
	m := triageModel(t)
	m.setCursor(ui.Row{Kind: ui.RowQuest, QuestID: "qb1"})
	m.openProjectPicker()

	if m.modal == nil || m.modal.Kind != ModalProjectPicker || m.modal.TakeUp {
		t.Fatalf("Ctrl+O should open the campaign picker in file-open mode, got %+v", m.modal)
	}
	m.modal.PickerIndex = pickerIndexFor(m.modal, "p1")
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})

	q := m.findQuest("qb1")
	if q.ProjectID != "p1" {
		t.Fatalf("quest should be filed to p1, got %q", q.ProjectID)
	}
	if q.BannerID != "b1" {
		t.Fatalf("filing should inherit the campaign's banner b1, got %q", q.BannerID)
	}
	if q.Status != model.StatusOpen {
		t.Fatalf("Ctrl+O files as open, got status %q", q.Status)
	}
}

// Ctrl+A on a Questboard quest opens the take-up picker; confirming files the
// quest into the campaign AND takes it up (active), so it reaches Camp.
func TestTriageTakeUpMakesActive(t *testing.T) {
	m := triageModel(t)
	m.setCursor(ui.Row{Kind: ui.RowQuest, QuestID: "qb1"})
	m.toggleActive()

	if m.modal == nil || m.modal.Kind != ModalProjectPicker || !m.modal.TakeUp {
		t.Fatalf("Ctrl+A on a Questboard quest should open the take-up picker, got %+v", m.modal)
	}
	m.modal.PickerIndex = pickerIndexFor(m.modal, "p1")
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})

	q := m.findQuest("qb1")
	if q.ProjectID != "p1" || q.BannerID != "b1" {
		t.Fatalf("take-up should file + inherit banner, got project=%q banner=%q", q.ProjectID, q.BannerID)
	}
	if q.Status != model.StatusActive {
		t.Fatalf("take-up should mark the quest active, got %q", q.Status)
	}
	if !m.toastActive() || !strings.Contains(m.toastText, "Migrate") {
		t.Fatalf("a destination toast should name the campaign, got active=%v text=%q", m.toastActive(), m.toastText)
	}
}

// The picker orders campaigns recents-first and annotates each with its active
// count.
func TestTriagePickerRecentsAndCounts(t *testing.T) {
	m := triageModel(t)
	mod := projectPickerModal(m.store, "qb1", "", false)

	// items[0] is always the "no campaign" (Questboard) escape hatch.
	if mod.PickerItems[0].ID != "" {
		t.Fatalf("first item should be the Questboard escape hatch, got %+v", mod.PickerItems[0])
	}
	// p1 (recently touched) must come before p2 (untouched).
	if pickerIndexFor(mod, "p1") >= pickerIndexFor(mod, "p2") {
		t.Fatalf("recently-touched p1 should sort before p2: p1=%d p2=%d", pickerIndexFor(mod, "p1"), pickerIndexFor(mod, "p2"))
	}
	if h := mod.PickerItems[pickerIndexFor(mod, "p1")].Hint; !strings.Contains(h, "1 active") {
		t.Fatalf("p1 should show its active count, got hint %q", h)
	}
	if h := mod.PickerItems[pickerIndexFor(mod, "p2")].Hint; h != "" {
		t.Fatalf("p2 has no active quests, so no count hint, got %q", h)
	}
	// Archived campaigns are not filing targets.
	m.store.Projects = append(m.store.Projects, model.Project{ID: "p3", Name: "Old", Archived: true})
	mod2 := projectPickerModal(m.store, "qb1", "", false)
	if pickerIndexFor(mod2, "p3") != -1 {
		t.Fatal("archived campaigns must not appear as filing targets")
	}
}
