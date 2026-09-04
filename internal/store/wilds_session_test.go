package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// A Save→Load→Save→Load round-trip must preserve every WildsSession field —
// this is persisted user data (the focus log), so nothing may be dropped.
func TestRoundTripPreservesWildsSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	start := time.Date(2026, 9, 3, 9, 15, 0, 0, time.UTC)
	s := &Store{
		Projects: []model.Project{{ID: "p1", Name: "Migrate"}},
		Quests:   []model.Quest{{ID: "q1", Title: "Fix the parser", ProjectID: "p1"}},
		WildsSessions: []model.WildsSession{
			{QuestID: "q1", Start: start, End: start.Add(24 * time.Minute), Completed: true},
			{QuestID: "q1", Start: start.Add(time.Hour), End: start.Add(time.Hour + 5*time.Minute)},
		},
	}

	for i := 0; i < 2; i++ {
		if err := Save(path, s); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
		loaded, err := Load(path)
		if err != nil {
			t.Fatalf("load %d: %v", i, err)
		}
		if len(loaded.WildsSessions) != 2 {
			t.Fatalf("round-trip %d dropped sessions: got %d", i, len(loaded.WildsSessions))
		}
		got := loaded.WildsSessions[0]
		if got.QuestID != "q1" || !got.Completed || !got.Start.Equal(start) || got.Duration() != 24*time.Minute {
			t.Fatalf("round-trip %d corrupted session 0: %+v", i, got)
		}
		if loaded.WildsSessions[1].Completed {
			t.Fatalf("round-trip %d flipped a bail into a completion", i)
		}
		s = loaded
	}
}

// Pre-focus-loop data (no wildsSessions key) loads with an empty log — the
// field is additive, so old data is untouched.
func TestLoadPreSessionData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	raw := `{"projects":[{"id":"p1","name":"Migrate"}],"quests":[{"id":"q1","title":"Fix","projectId":"p1"}]}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.WildsSessions) != 0 {
		t.Fatalf("pre-session data should load with no sessions, got %d", len(loaded.WildsSessions))
	}
}
