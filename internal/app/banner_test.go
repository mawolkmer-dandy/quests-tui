package app

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func bannerModel(t *testing.T) *Model {
	ui.Init(true)
	return &Model{
		store: &store.Store{
			Banners:  []model.Banner{{ID: "b1", Name: "Platform"}},
			Projects: []model.Project{{ID: "p1", Name: "Migrate"}},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{},
		sectionScroll:     map[string]int{},
		sectionMaxScroll:  map[string]int{},
	}
}

// The banner picker (a ModalProjectPicker with TargetProjectID) assigns the
// chosen banner to the campaign, and can clear it back to ungrouped.
func TestBannerPickerAssignsCampaign(t *testing.T) {
	m := bannerModel(t)
	m.width, m.height = 100, 30

	m.modal = bannerPickerModal(m.store, "p1", "") // items: [— no banner —, Platform]
	m.modal.PickerIndex = 1                        // Platform
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.findProject("p1").BannerID; got != "b1" {
		t.Fatalf("campaign should fly under banner b1, got %q", got)
	}

	m.modal = bannerPickerModal(m.store, "p1", "b1")
	m.modal.PickerIndex = 0 // — no banner —
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.findProject("p1").BannerID; got != "" {
		t.Fatalf("campaign should be ungrouped again, got %q", got)
	}
}

// Activating "+ New Banner" in the hall creates a banner and dives into the
// pane with its name open for inline editing (mirrors "+ New Campaign").
func TestCreateBanner(t *testing.T) {
	m := bannerModel(t)
	m.width, m.height = 100, 30
	before := len(m.store.Banners)
	m.hallCursor = cursorTarget{kind: ui.RowNewBanner}
	m.activateHallEntry()
	if len(m.store.Banners) != before+1 {
		t.Fatalf("+ New Banner should add a banner: %d → %d", before, len(m.store.Banners))
	}
	if m.hallFocus {
		t.Fatal("creating a banner should dive into its pane to name it")
	}
	if m.cursor.kind != ui.RowBanner {
		t.Fatalf("pane cursor should land on the new banner header, got %v", m.cursor.kind)
	}
	if m.editor == nil {
		t.Fatal("the new banner should open in inline-edit mode")
	}
}

// A loose quest (banner, no campaign) renders under its banner in the hall and,
// when active, is eligible for Camp/Wilds — but never counts as Questboard.
func TestLooseQuestUnderBanner(t *testing.T) {
	m := bannerModel(t) // banner b1, campaign p1
	m.store.Quests = append(m.store.Quests, model.Quest{ID: "lq", Title: "On-call", BannerID: "b1", Status: model.StatusActive})

	rows := ui.SectionContent(m.store, "campaigns", m.collapsedProjects)
	bIdx, qIdx := -1, -1
	for i, r := range rows {
		if r.Kind == ui.RowBanner && r.BannerID == "b1" {
			bIdx = i
		}
		if r.Kind == ui.RowQuest && r.QuestID == "lq" {
			qIdx = i
		}
	}
	if bIdx < 0 || qIdx <= bIdx {
		t.Fatalf("loose quest should render under its banner (banner=%d, quest=%d)", bIdx, qIdx)
	}
	if _, ok := m.wildsEligible()["lq"]; !ok {
		t.Fatal("an active loose quest should be eligible for Camp")
	}
	if m.findQuest("lq").InQuestboard() {
		t.Fatal("a loose quest must not count as Questboard")
	}
}

func hallHasProject(m *Model, id string) bool {
	for _, r := range ui.SectionContent(m.store, "campaigns", m.collapsedProjects) {
		if r.Kind == ui.RowProject && r.ProjectID == id {
			return true
		}
	}
	return false
}

func vaultHasCampaign(m *Model, id string) bool {
	for _, r := range ui.SectionContent(m.store, "someday", m.collapsedProjects) {
		if r.Kind == ui.RowVaultCampaign && r.ProjectID == id {
			return true
		}
	}
	return false
}

// Ctrl+D marks a campaign done but it STAYS in the hall (nothing auto-vaults);
// Ctrl+V is what sends it to the Vault, manually.
func TestCompleteCampaignStaysUntilVaulted(t *testing.T) {
	m := bannerModel(t) // p1 "Migrate", ungrouped
	m.width, m.height = 100, 30

	m.setCursor(ui.Row{Kind: ui.RowProject, ProjectID: "p1"})
	m.toggleDone()
	if !m.findProject("p1").IsCompleted() {
		t.Fatal("Ctrl+D should mark the campaign done")
	}
	if !hallHasProject(m, "p1") {
		t.Fatal("a done campaign should stay in the hall until vaulted")
	}
	if vaultHasCampaign(m, "p1") {
		t.Fatal("a done campaign must not auto-move to the Vault")
	}

	m.setCursor(ui.Row{Kind: ui.RowProject, ProjectID: "p1"})
	m.toggleVault()
	if hallHasProject(m, "p1") {
		t.Fatal("Ctrl+V should move the campaign out of the hall")
	}
	if !vaultHasCampaign(m, "p1") {
		t.Fatal("Ctrl+V should move the campaign into the Vault")
	}
}
