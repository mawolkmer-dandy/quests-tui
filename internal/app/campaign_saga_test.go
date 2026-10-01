package app

import (
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func sagaModel(t *testing.T, ps ...model.Project) *Model {
	ui.Init(true)
	return &Model{
		store:             &store.Store{Projects: ps},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{},
		sectionScroll:     map[string]int{},
		sectionMaxScroll:  map[string]int{},
	}
}

// setNextChapter links a campaign to its next chapter and derives the previous
// one, but refuses a self-link or one that would close a cycle.
func TestSagaLinkIntegrity(t *testing.T) {
	m := sagaModel(t,
		model.Project{ID: "c1", Name: "Ch.1"},
		model.Project{ID: "c2", Name: "Ch.2"},
		model.Project{ID: "c3", Name: "Ch.3"},
	)
	c1, c2, c3 := &m.store.Projects[0], &m.store.Projects[1], &m.store.Projects[2]

	if m.setNextChapter(c1, "c1") {
		t.Fatal("a campaign must not link to itself")
	}
	if c1.NextID != "" {
		t.Fatalf("self-link should be a no-op, got NextID=%q", c1.NextID)
	}

	if !m.setNextChapter(c1, "c2") || c1.NextID != "c2" {
		t.Fatalf("c1 → c2 should link, got NextID=%q", c1.NextID)
	}
	if !m.setNextChapter(c2, "c3") || c2.NextID != "c3" {
		t.Fatalf("c2 → c3 should link, got NextID=%q", c2.NextID)
	}

	// c3 → c1 would close the loop c1→c2→c3→c1 — rejected.
	if m.setNextChapter(c3, "c1") {
		t.Fatal("linking c3 → c1 should be refused (cycle)")
	}
	if c3.NextID != "" {
		t.Fatalf("cycle link should be a no-op, got NextID=%q", c3.NextID)
	}

	// Previous chapter is derived by scanning for whoever points here.
	if prev := m.prevChapter("c2"); prev == nil || prev.ID != "c1" {
		t.Fatalf("prevChapter(c2) should be c1, got %+v", prev)
	}
	if prev := m.prevChapter("c1"); prev != nil {
		t.Fatalf("c1 heads the saga — no previous chapter, got %+v", prev)
	}

	// Clearing the link is always allowed.
	if !m.setNextChapter(c1, "") || c1.NextID != "" {
		t.Fatalf("clearing c1's link should succeed, got NextID=%q", c1.NextID)
	}
}

// The saga picker offers the other live campaigns, minus any that would form a
// cycle (and minus the campaign itself and archived ones).
func TestSagaPickerExcludesCycleAndSelf(t *testing.T) {
	m := sagaModel(t,
		model.Project{ID: "c1", Name: "Ch.1", NextID: "c2"},
		model.Project{ID: "c2", Name: "Ch.2"},
		model.Project{ID: "c3", Name: "Ch.3", Archived: true},
	)
	// Picking c2's next chapter: c1 would cycle (c1→c2→c1), c2 is self, c3 is
	// archived — so only "— no next chapter —" remains.
	items := m.sagaPickerModal("c2").PickerItems
	for _, it := range items {
		if it.ID == "c1" || it.ID == "c2" || it.ID == "c3" {
			t.Fatalf("picker should exclude cycle/self/archived, got item %q", it.ID)
		}
	}
	if len(items) != 1 || items[0].ID != "" {
		t.Fatalf("only the clear option should remain, got %+v", items)
	}
}

// A campaign in the middle of a saga renders both chapter links at the bottom of
// its pane, each drilling into the linked campaign.
func TestSagaNavRowsRenderAndDrill(t *testing.T) {
	m := sagaModel(t,
		model.Project{ID: "c1", Name: "Ch.1", NextID: "c2"},
		model.Project{ID: "c2", Name: "Ch.2", NextID: "c3"},
		model.Project{ID: "c3", Name: "Ch.3"},
	)
	rows := m.campaignPaneRows("c2")
	var from, in *ui.Row
	for i := range rows {
		if rows[i].Kind != ui.RowSagaLink {
			continue
		}
		if rows[i].ProjectID == "c1" {
			from = &rows[i]
		}
		if rows[i].ProjectID == "c3" {
			in = &rows[i]
		}
	}
	if from == nil || in == nil {
		t.Fatalf("Ch.2 should show both chapter links (from c1, in c3): %+v", rows)
	}

	// Drilling a saga link jumps the hall/pane to that campaign.
	m.hallCursor = cursorTarget{kind: ui.RowProject, projectID: "c2"}
	m.setCursor(*in)
	m.handleReveal() // Tab
	if m.hallCursor.projectID != "c3" {
		t.Fatalf("drilling '→ Continues in' should show c3, got %q", m.hallCursor.projectID)
	}
}
