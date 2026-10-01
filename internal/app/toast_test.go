package app

import (
	"strings"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Status-line notices queue rather than replace: a second one shown while the
// first is still animating waits its turn, then plays; when the queue drains,
// the slot goes idle (returning to its default).
func TestToastQueuePlaysSequentially(t *testing.T) {
	ui.Init(true)
	m := &Model{}

	if cmd := m.showClipboardToastText("first"); cmd == nil {
		t.Fatal("first toast should start (a non-nil tick)")
	}
	if m.toastPhase != toastEnter || m.toastText != "first" {
		t.Fatalf("first should be typing in, got phase=%d text=%q", m.toastPhase, m.toastText)
	}

	// A second, shown mid-animation, queues without replacing the first.
	if cmd := m.showClipboardToastText("second"); cmd != nil {
		t.Fatal("a second toast while animating should enqueue (nil), not restart")
	}
	if m.toastText != "first" || len(m.toastQueue) != 1 {
		t.Fatalf("first should still show, second queued: text=%q queue=%v", m.toastText, m.toastQueue)
	}

	// Step frames until the first finishes and the second takes over.
	step := func(untilText string) bool {
		total := toastEnterFrames + toastHoldFrames + toastExitFrames + 2
		for i := 0; i < total; i++ {
			m.advanceToast(m.toastGen)
			if m.toastText == untilText || m.toastPhase == toastIdle {
				return true
			}
		}
		return false
	}
	step("second")
	if m.toastText != "second" || m.toastPhase != toastEnter {
		t.Fatalf("after the first, the second should start, got phase=%d text=%q", m.toastPhase, m.toastText)
	}

	// Draining the second empties the queue → idle → default shown.
	step("")
	if m.toastPhase != toastIdle || m.toastActive() {
		t.Fatalf("queue drained should go idle, got phase=%d active=%v", m.toastPhase, m.toastActive())
	}
	if m.renderToast() != "" {
		t.Fatalf("idle toast should render nothing, got %q", m.renderToast())
	}
}

// The same notice, fired repeatedly while it's showing or already queued, is not
// re-added — a runaway trigger (a drag-select firing thousands of copies) can't
// pile up identical toasts that then drain one at a time.
func TestToastDedupes(t *testing.T) {
	ui.Init(true)
	m := &Model{}

	if cmd := m.showClipboardToastText("copied to clipboard"); cmd == nil {
		t.Fatal("first toast should start")
	}
	// Flood the SAME message while it's animating — none should queue.
	for i := 0; i < 10000; i++ {
		m.showClipboardToastText("copied to clipboard")
	}
	if len(m.toastQueue) != 0 {
		t.Fatalf("duplicates of the showing toast must not queue, got %d", len(m.toastQueue))
	}

	// A different message queues once; repeats of it are dropped.
	m.showClipboardToastText("Deleted quest")
	for i := 0; i < 100; i++ {
		m.showClipboardToastText("Deleted quest")
	}
	if len(m.toastQueue) != 1 {
		t.Fatalf("a distinct message should queue exactly once, got %d", len(m.toastQueue))
	}
}

// The type-in reveals a growing prefix of the text.
func TestToastTypesInPrefix(t *testing.T) {
	ui.Init(true)
	m := &Model{}
	m.showClipboardToastText("Deleted quest")
	m.toastFrame = 1 // early in the type-in
	early := m.renderToast()
	m.toastFrame = toastEnterFrames // fully typed in
	full := m.renderToast()
	if !strings.Contains(full, "Deleted quest") {
		t.Fatalf("fully typed-in should show the whole text, got %q", full)
	}
	if len([]rune(stripANSI(early))) >= len([]rune(stripANSI(full))) {
		t.Fatalf("early frame should show less than the full text: early=%q full=%q", early, full)
	}
}
