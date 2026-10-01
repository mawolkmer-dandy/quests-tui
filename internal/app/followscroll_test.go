package app

import "testing"

// followScroll keeps a one-line margin from the "…" more-indicator that occupies
// the first/last visible line when scrolled, so the cursor is never stranded
// behind it — while still releasing that margin at the true top/bottom, where no
// ellipsis is drawn. This is what let a scrolled Vault reach its top row again.
func TestFollowScrollRevealsEdges(t *testing.T) {
	const total, viewH = 40, 10

	// Scrolled down, then the cursor walks up to the topmost selectable row (row 1,
	// row 0 being a non-selectable day header): scroll must reach 0 so the header
	// and first row show, rather than parking the cursor under the top "…".
	if got := followScroll(total, 1, 5, viewH, true); got != 0 {
		t.Errorf("following up to the top row should scroll to 0, got %d", got)
	}
	// The very first row reaches the top too.
	if got := followScroll(total, 0, 8, viewH, true); got != 0 {
		t.Errorf("following to row 0 should scroll to 0, got %d", got)
	}
	// The last row reaches the bottom (scroll = total-viewH), so it isn't hidden
	// behind the bottom "…".
	if got := followScroll(total, total-1, 0, viewH, true); got != total-viewH {
		t.Errorf("following to the last row should scroll to %d, got %d", total-viewH, got)
	}

	// Mid-list, the cursor stays one line inside each edge (off the ellipsis rows).
	// Cursor at row 20 with scroll 20 would sit on the top ellipsis → nudge up.
	if got := followScroll(total, 20, 20, viewH, true); got > 19 {
		t.Errorf("cursor should stay below the top ellipsis, got scroll %d (cursor at top edge)", got)
	}

	// Wheel scrolling (follow=false) only clamps, never moves the cursor margin.
	if got := followScroll(total, -1, 100, viewH, false); got != total-viewH {
		t.Errorf("non-following scroll should clamp to max, got %d", got)
	}
}
