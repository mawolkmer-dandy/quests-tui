package app

import (
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func escNavModel(t *testing.T) *Model {
	ui.Init(true)
	m := &Model{
		store: &store.Store{
			Banners:  []model.Banner{{ID: "b1", Name: "Platform"}},
			Projects: []model.Project{{ID: "p1", Name: "Migrate", BannerID: "b1"}},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}
	m.width, m.height = 120, 40
	return m
}

// Esc walks the hierarchy: inside a campaign it steps up to that campaign's
// banner view; a second Esc from the banner focuses the sidebar.
func TestEscCampaignToBannerToSidebar(t *testing.T) {
	m := escNavModel(t)
	m.hallCursor = cursorTarget{kind: ui.RowProject, projectID: "p1"}
	m.hallFocus = false // inside the campaign pane

	m.escFromPane()
	if m.hallFocus {
		t.Fatal("Esc from a campaign should land in the banner view, not the sidebar")
	}
	if m.hallCursor.kind != ui.RowBanner || m.hallCursor.bannerID != "b1" {
		t.Fatalf("Esc from a campaign should select its parent banner, got %+v", m.hallCursor)
	}

	m.escFromPane()
	if !m.hallFocus {
		t.Fatal("Esc from the banner view should focus the sidebar")
	}
}

// Esc from a room (not a campaign) goes straight to the sidebar.
func TestEscRoomToSidebar(t *testing.T) {
	m := escNavModel(t)
	m.hallCursor = cursorTarget{kind: ui.RowSection, section: "inbox"}
	m.hallFocus = false

	m.escFromPane()
	if !m.hallFocus {
		t.Fatal("Esc from a room should go straight to the sidebar")
	}
}
