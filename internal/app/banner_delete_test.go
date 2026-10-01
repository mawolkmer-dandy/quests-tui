package app

import (
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Deleting a banner ungroups its campaigns and loose quests (their work
// survives — campaigns to Unassigned, loose quests to the Questboard) rather
// than deleting them, and reports how many moved.
func TestDeleteBannerUngroupsContents(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store: &store.Store{
			Banners: []model.Banner{{ID: "b1", Name: "Platform"}, {ID: "b2", Name: "Home"}},
			Projects: []model.Project{
				{ID: "c1", Name: "Migrate", BannerID: "b1"},
				{ID: "c2", Name: "Other", BannerID: "b1"},
				{ID: "c3", Name: "Elsewhere", BannerID: "b2"},
			},
			Quests: []model.Quest{
				{ID: "lq1", Title: "loose", BannerID: "b1"},       // loose under b1
				{ID: "q1", Title: "in campaign", ProjectID: "c1"}, // inside a b1 campaign
			},
		},
		path: filepath.Join(t.TempDir(), "data.json"),
	}

	n := m.deleteBannerByID("b1")
	if n != 3 { // c1, c2, lq1
		t.Fatalf("ungrouped count = %d, want 3 (c1, c2, lq1)", n)
	}
	if len(m.store.Banners) != 1 || m.store.Banners[0].ID != "b2" {
		t.Fatalf("only b2 should remain: %+v", m.store.Banners)
	}
	// Campaigns + quests survive; b1's are ungrouped, b2's untouched.
	if len(m.store.Projects) != 3 || len(m.store.Quests) != 2 {
		t.Fatalf("no campaign/quest should be deleted: %d projects, %d quests", len(m.store.Projects), len(m.store.Quests))
	}
	byID := map[string]string{}
	for _, p := range m.store.Projects {
		byID[p.ID] = p.BannerID
	}
	if byID["c1"] != "" || byID["c2"] != "" {
		t.Fatalf("b1 campaigns should be ungrouped: c1=%q c2=%q", byID["c1"], byID["c2"])
	}
	if byID["c3"] != "b2" {
		t.Fatalf("b2's campaign must be untouched, got %q", byID["c3"])
	}
	for _, q := range m.store.Quests {
		if q.ID == "lq1" && q.BannerID != "" {
			t.Fatalf("loose quest should be ungrouped, got banner %q", q.BannerID)
		}
	}
}
