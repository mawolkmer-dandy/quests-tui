package app

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func movePickerModel(t *testing.T) *Model {
	ui.Init(true)
	return &Model{
		store: &store.Store{
			Banners:  []model.Banner{{ID: "b1", Name: "Platform"}},
			Projects: []model.Project{{ID: "p1", Name: "Migrate", BannerID: "b1"}},
			Quests:   []model.Quest{{ID: "q", Title: "Review a PR"}}, // a Questboard notice
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}
}

// The move picker offers areas (Errands + banners) as targets, and choosing one
// makes the quest a loose quest under that area (no campaign).
func TestMovePickerToArea(t *testing.T) {
	m := movePickerModel(t)

	mod := projectPickerModal(m.store, "q", "", false)
	// It lists the campaign AND the areas (Errands + Platform).
	var haveCampaign, haveErrands, havePlatform bool
	for _, it := range mod.PickerItems {
		switch it.ID {
		case "p1":
			haveCampaign = true
		case bannerMoveTarget + errandsBanner:
			haveErrands = true
		case bannerMoveTarget + "b1":
			havePlatform = true
		}
	}
	if !haveCampaign || !haveErrands || !havePlatform {
		t.Fatalf("picker should list campaign + Errands + banners: campaign=%v errands=%v platform=%v", haveCampaign, haveErrands, havePlatform)
	}

	// Select the Platform area and confirm.
	for i, it := range mod.PickerItems {
		if it.ID == bannerMoveTarget+"b1" {
			mod.PickerIndex = i
		}
	}
	m.modal = mod
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})

	q := m.findQuest("q")
	if q.ProjectID != "" || q.BannerID != "b1" {
		t.Fatalf("moving to an area should make it a loose quest under the banner, got projectID=%q bannerID=%q", q.ProjectID, q.BannerID)
	}
}

// Moving to Errands files the quest under the pinned Errands area.
func TestMovePickerToErrands(t *testing.T) {
	m := movePickerModel(t)
	mod := projectPickerModal(m.store, "q", "", false)
	for i, it := range mod.PickerItems {
		if it.ID == bannerMoveTarget+errandsBanner {
			mod.PickerIndex = i
		}
	}
	m.modal = mod
	m.updateModal(tea.KeyPressMsg{Code: tea.KeyEnter})

	if q := m.findQuest("q"); q.BannerID != errandsBanner || q.ProjectID != "" {
		t.Fatalf("moving to Errands should file it under the errands banner, got projectID=%q bannerID=%q", q.ProjectID, q.BannerID)
	}
}

// A loose quest's current area is pre-selected when the picker opens.
func TestMovePickerPreselectsArea(t *testing.T) {
	m := movePickerModel(t)
	m.findQuest("q").BannerID = "b1" // already loose under Platform
	m.cursor = cursorTarget{kind: ui.RowQuest, questID: "q"}
	m.openProjectPicker()

	sel := m.modal.PickerItems[m.modal.PickerIndex].ID
	if sel != bannerMoveTarget+"b1" {
		t.Fatalf("the quest's current area should be pre-selected, got %q", sel)
	}
}
