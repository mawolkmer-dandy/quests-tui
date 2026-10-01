package store

import (
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// A campaign's inline notes (Project.Body) — multi-line, every BodyLine field
// populated — survive Save → Load → Save → Load with nothing dropped. Campaign
// notes are a persisted field edited inline in the Tavern pane, so a round-trip
// regression here would silently lose a user's writing.
func TestRoundTripPreservesCampaignNotes(t *testing.T) {
	orig := &Store{
		Projects: []model.Project{
			{
				ID:       "p1",
				Name:     "Migrate scan-stream",
				Priority: model.PriorityHigh,
				Body: []model.BodyLine{
					{ID: "n1", Text: "Owner: me. Cutover before Q3."},
					{ID: "n2", Text: ""}, // an intentional blank paragraph break
					{ID: "n3", Text: "- a bulleted note", Done: true, Indent: 1},
					{ID: "n4", Text: "Spec: docs.google.com/document/d/1Fmbg…/edit"},
				},
				BodyLinks: map[string]string{
					"docs.google.com/document/d/1Fmbg…/edit": "https://docs.google.com/document/d/1FmbgLVtTtByAbCdEfGh/edit?tab=t.0",
				},
				NextID: "p2", // a saga link to the next chapter
			},
			{ID: "p2", Name: "No notes"}, // nil body must stay absent, not become []
		},
		Quests: []model.Quest{
			{ID: "q1", Title: "Fix the parser", ProjectID: "p1", Status: model.StatusActive},
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

	proj := map[string]model.Project{}
	for _, p := range s.Projects {
		proj[p.ID] = p
	}

	got := proj["p1"].Body
	if len(got) != 4 {
		t.Fatalf("campaign notes line count = %d, want 4: %+v", len(got), got)
	}
	want := orig.Projects[0].Body
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("note line %d changed:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}

	full := "https://docs.google.com/document/d/1FmbgLVtTtByAbCdEfGh/edit?tab=t.0"
	if proj["p1"].BodyLinks["docs.google.com/document/d/1Fmbg…/edit"] != full {
		t.Errorf("shortened-link → full-URL mapping lost: %+v", proj["p1"].BodyLinks)
	}
	if proj["p1"].NextID != "p2" {
		t.Errorf("saga NextID link lost: %q", proj["p1"].NextID)
	}
	if proj["p1"].Priority != model.PriorityHigh {
		t.Errorf("campaign priority lost: %q", proj["p1"].Priority)
	}

	if proj["p2"].Body != nil {
		t.Errorf("a campaign with no notes gained a body: %+v", proj["p2"].Body)
	}
}
