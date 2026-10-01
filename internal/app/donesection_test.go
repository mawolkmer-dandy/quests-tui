package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// headerBefore returns the last RowDayHeader label at or above the row matching
// pred, so we can assert which group a row falls under.
func headerBefore(rows []ui.Row, pred func(ui.Row) bool) string {
	label := ""
	for _, r := range rows {
		if r.Kind == ui.RowDayHeader {
			label = r.Label
		}
		if pred(r) {
			return label
		}
	}
	return "<not found>"
}

// A completed campaign lands under a "Done" group (not "Later") in the banner
// pane; an open one stays under "Later".
func TestBannerPaneDoneCampaigns(t *testing.T) {
	ui.Init(true)
	now := time.Now()
	m := &Model{
		store: &store.Store{
			Banners: []model.Banner{{ID: "b1", Name: "DUI"}},
			Projects: []model.Project{
				{ID: "act", Name: "Active", BannerID: "b1"},
				{ID: "open", Name: "Open", BannerID: "b1"},
				{ID: "done", Name: "Done one", BannerID: "b1", CompletedAt: &now},
			},
			Quests: []model.Quest{
				{ID: "q", Title: "work", ProjectID: "act", Status: model.StatusActive}, // makes "act" active
			},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}

	rows := m.bannerPaneRows("b1")
	if got := headerBefore(rows, func(r ui.Row) bool { return r.Kind == ui.RowProject && r.ProjectID == "done" }); got != "Done" {
		t.Errorf("a completed campaign should be under 'Done', got %q", got)
	}
	if got := headerBefore(rows, func(r ui.Row) bool { return r.Kind == ui.RowProject && r.ProjectID == "open" }); got != "Later" {
		t.Errorf("an open campaign should be under 'Later', got %q", got)
	}
}

// A done quest lands under a "Done" group in a campaign's own pane.
func TestCampaignPaneDoneQuests(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store: &store.Store{
			Projects: []model.Project{{ID: "p1", Name: "Campaign"}},
			Quests: []model.Quest{
				{ID: "a", Title: "Active", ProjectID: "p1", Status: model.StatusActive},
				{ID: "o", Title: "Open", ProjectID: "p1", Status: model.StatusOpen},
				{ID: "d", Title: "Done", ProjectID: "p1", Status: model.StatusDone},
			},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}

	rows := m.campaignPaneRows("p1")
	if got := headerBefore(rows, func(r ui.Row) bool { return r.Kind == ui.RowQuest && r.QuestID == "d" }); got != "Done" {
		t.Errorf("a done quest should be under 'Done', got %q", got)
	}
	if got := headerBefore(rows, func(r ui.Row) bool { return r.Kind == ui.RowQuest && r.QuestID == "o" }); got != "Later" {
		t.Errorf("an open quest should be under 'Later', got %q", got)
	}
}
