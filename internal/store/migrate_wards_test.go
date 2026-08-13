package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateWardsToTracksAndLookouts verifies that a quest saved with the
// deprecated `wards` field (combined event + dashboard) migrates on load into
// the split Tracks (events + marks) and Lookouts (dashboards), losing nothing,
// and that the vestigial field is cleared.
func TestMigrateWardsToTracksAndLookouts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	legacy := `{
	  "projects": [],
	  "quests": [{
	    "id": "q1",
	    "title": "Ship it",
	    "body": [],
	    "wards": [
	      {"event": "Practice - Checkout - Case Loaded", "marks": ["source", "count"], "dashboard": "https://app.amplitude.com/x/chart/abc", "tool": "amplitude"},
	      {"event": "Global - Order Placed"}
	    ]
	  }]
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s.Quests) != 1 {
		t.Fatalf("expected 1 quest, got %d", len(s.Quests))
	}
	q := s.Quests[0]

	if len(q.Tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d: %+v", len(q.Tracks), q.Tracks)
	}
	if q.Tracks[0].Event != "Practice - Checkout - Case Loaded" || len(q.Tracks[0].Marks) != 2 {
		t.Fatalf("track 0 mismigrated: %+v", q.Tracks[0])
	}
	if q.Tracks[1].Event != "Global - Order Placed" {
		t.Fatalf("track 1 mismigrated: %+v", q.Tracks[1])
	}
	if len(q.Lookouts) != 1 {
		t.Fatalf("expected 1 lookout (only the ward with a dashboard), got %d: %+v", len(q.Lookouts), q.Lookouts)
	}
	if q.Lookouts[0].URL != "https://app.amplitude.com/x/chart/abc" || q.Lookouts[0].Tool != "amplitude" {
		t.Fatalf("lookout mismigrated: %+v", q.Lookouts[0])
	}
	if q.Wards != nil {
		t.Fatalf("deprecated Wards should be cleared after migration, got %+v", q.Wards)
	}
}
