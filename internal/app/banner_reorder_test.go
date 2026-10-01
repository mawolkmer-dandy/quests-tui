package app

import (
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Shift+↑/↓ on a hall banner swaps it with its neighbor in the sidebar order,
// keeping the selection on it; it's a no-op at the ends and on the synthetic
// "Unassigned" area.
func TestMoveBannerReorders(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store: &store.Store{Banners: []model.Banner{
			{ID: "b1", Name: "A"}, {ID: "b2", Name: "B"}, {ID: "b3", Name: "C"},
		}},
		path: filepath.Join(t.TempDir(), "data.json"),
	}
	order := func() []string {
		out := make([]string, len(m.store.Banners))
		for i, b := range m.store.Banners {
			out[i] = b.Name
		}
		return out
	}
	eq := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	m.hallCursor = cursorTarget{kind: ui.RowBanner, bannerID: "b1"}
	m.moveBanner(1) // A down past B
	if !eq(order(), []string{"B", "A", "C"}) {
		t.Fatalf("after move-down: %v", order())
	}
	if m.hallCursor.bannerID != "b1" {
		t.Fatal("selection should stay on the moved banner")
	}

	m.moveBanner(-1) // A back up
	if !eq(order(), []string{"A", "B", "C"}) {
		t.Fatalf("after move-up: %v", order())
	}

	m.moveBanner(-1) // already at the top — no-op
	if !eq(order(), []string{"A", "B", "C"}) {
		t.Fatalf("move-up at the top should be a no-op: %v", order())
	}

	m.hallCursor.bannerID = "" // the synthetic "Unassigned" area
	m.moveBanner(1)
	if !eq(order(), []string{"A", "B", "C"}) {
		t.Fatalf("Unassigned isn't reorderable: %v", order())
	}
}
