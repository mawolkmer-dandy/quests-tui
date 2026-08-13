package app

import (
	"strings"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func TestAgeDaysLabel(t *testing.T) {
	ui.Init(true)
	if got := ageDaysLabel(-1, true); got != "" {
		t.Fatalf("unknown age should render empty, got %q", got)
	}
	if !strings.Contains(stripANSI(ageDaysLabel(32, true)), "32d") {
		t.Fatal("age label must show the day count")
	}
	for _, d := range []int{5, 32, 120} {
		if strings.Contains(ageDaysLabel(d, true), "delete") {
			t.Fatalf("age label must not contain a delete prompt (days=%d)", d)
		}
	}
	fresh, yellow, red := ageDaysLabel(10, true), ageDaysLabel(45, true), ageDaysLabel(120, true)
	if fresh == yellow || yellow == red || fresh == red {
		t.Fatalf("flag age tints must differ by threshold:\nfresh=%q\n>30=%q\n>90=%q", fresh, yellow, red)
	}
	// Non-flags (colored=false) are plain muted at EVERY age — colored differs
	// from plain past the thresholds, but plain never changes with age.
	if ageDaysLabel(120, false) != "  "+ui.StyleMuted.Render("120d") {
		t.Fatalf("non-flag age must be plain muted, got %q", ageDaysLabel(120, false))
	}
	if ageDaysLabel(120, true) == ageDaysLabel(120, false) {
		t.Fatal("a stale flag must be tinted differently from a plain (non-flag) age")
	}
}

func TestTrackWordNoInProd(t *testing.T) {
	m := &Model{prStatus: map[string]PRStatus{"#1": {Code: "#1", Status: "merged"}}}
	w := m.trackWord(model.Track{Event: "E", Marks: []string{"a", "b"}, SourcePR: "#1"})
	if strings.Contains(w, "in prod") {
		t.Fatalf("track word should not say 'in prod': %q", w)
	}
	if !strings.Contains(w, "marks") {
		t.Fatalf("track word should still show marks: %q", w)
	}
}
