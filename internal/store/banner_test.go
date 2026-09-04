package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// A store with banners, banner-grouped + completed campaigns, and a loose
// under-banner quest survives Save → Load → Save → Load with nothing dropped.
func TestRoundTripPreservesBanners(t *testing.T) {
	done := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	orig := &Store{
		Banners: []model.Banner{
			{ID: "b1", Name: "Platform", Icon: "⚑"},
			{ID: "b2", Name: "Home"},
		},
		Projects: []model.Project{
			{ID: "p1", Name: "Migrate scan-stream", BannerID: "b1"},
			{ID: "p2", Name: "Old launch", BannerID: "b1", CompletedAt: &done},
			{ID: "p3", Name: "ungrouped campaign"}, // no banner
		},
		Quests: []model.Quest{
			{ID: "q1", Title: "Fix the parser", ProjectID: "p1", Status: model.StatusActive},
			{ID: "q2", Title: "Answer the on-call page", BannerID: "b1"}, // loose under banner
			{ID: "q3", Title: "Unsorted"},                                // questboard
		},
	}
	path := filepath.Join(t.TempDir(), "data.json")

	if err := Save(path, orig); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, loaded); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(s.Banners) != 2 || s.Banners[0].Name != "Platform" || s.Banners[0].Icon != "⚑" || s.Banners[1].Name != "Home" {
		t.Fatalf("banners dropped/changed: %+v", s.Banners)
	}
	proj := map[string]model.Project{}
	for _, p := range s.Projects {
		proj[p.ID] = p
	}
	if proj["p1"].BannerID != "b1" {
		t.Fatalf("p1 bannerID lost: %q", proj["p1"].BannerID)
	}
	if p2 := proj["p2"]; !p2.IsCompleted() || p2.CompletedAt == nil || !p2.CompletedAt.Equal(done) {
		t.Fatalf("p2 completion lost: %+v", p2.CompletedAt)
	}
	if proj["p3"].BannerID != "" {
		t.Fatalf("p3 should stay ungrouped, got %q", proj["p3"].BannerID)
	}
	quest := map[string]model.Quest{}
	for _, q := range s.Quests {
		quest[q.ID] = q
	}
	if quest["q2"].BannerID != "b1" {
		t.Fatalf("q2 bannerID lost: %q", quest["q2"].BannerID)
	}
	if q2 := quest["q2"]; q2.InQuestboard() {
		t.Fatal("a loose-under-banner quest must not count as Questboard")
	}
	if q3 := quest["q3"]; !q3.InQuestboard() {
		t.Fatal("q3 (no project, no banner) should be in the Questboard")
	}
}

// Data written before Banners existed loads cleanly: no banners, campaigns
// ungrouped and not completed, existing quests preserved, no error.
func TestLoadPreBannerData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	old := `{
	  "projects": [{"id":"p1","name":"Homestead","archived":false,"body":[]}],
	  "quests": [{"id":"q1","title":"Repair the phial","type":"main","status":"","projectId":"p1","body":[],"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]
	}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Banners) != 0 {
		t.Fatalf("pre-banner data should have no banners, got %d", len(s.Banners))
	}
	if s.Projects[0].BannerID != "" || s.Projects[0].IsCompleted() {
		t.Fatal("pre-banner campaign should be ungrouped and not completed")
	}
	if s.Quests[0].BannerID != "" || s.Quests[0].Title != "Repair the phial" {
		t.Fatal("pre-banner quest data lost")
	}
}

// Data that ALREADY carries the new banner fields (not just our own Save output)
// parses correctly — the migration-against-new-format check.
func TestLoadNewFormatBannerData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	newFmt := `{
	  "banners": [{"id":"b1","name":"Platform","icon":"⚑"}],
	  "projects": [{"id":"p1","name":"Migrate","bannerId":"b1","completedAt":"2026-09-01T10:00:00Z","body":[]}],
	  "quests": [{"id":"q1","title":"Loose","bannerId":"b1","body":[]}]
	}`
	if err := os.WriteFile(path, []byte(newFmt), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Banners) != 1 || s.Banners[0].Name != "Platform" || s.Banners[0].Icon != "⚑" {
		t.Fatalf("banner not loaded: %+v", s.Banners)
	}
	if s.Projects[0].BannerID != "b1" || !s.Projects[0].IsCompleted() {
		t.Fatal("campaign banner/completion not loaded")
	}
	if s.Quests[0].BannerID != "b1" || s.Quests[0].InQuestboard() {
		t.Fatal("loose under-banner quest not loaded correctly")
	}
}
