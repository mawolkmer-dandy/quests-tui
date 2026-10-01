package app

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

const warningDuration = 2 * time.Second

// warningExpireMsg carries the generation it was scheduled for, so a stale
// timer from an earlier warning can't clear a newer one that replaced it
// before the first one expired.
type warningExpireMsg struct{ gen int }

// showWarning displays text next to target (in place of its title, like the
// delete y/n prompt) for warningDuration — used for "vault is read-only"
// when an action is blocked rather than silently doing nothing.
func (m *Model) showWarning(target cursorTarget, text string) tea.Cmd {
	m.warningGen++
	gen := m.warningGen
	m.warningTarget = target
	m.warningText = text
	return tea.Tick(warningDuration, func(time.Time) tea.Msg { return warningExpireMsg{gen: gen} })
}

func (m *Model) clearWarningIfCurrent(gen int) {
	if gen == m.warningGen {
		m.warningText = ""
	}
}

// The status-line toast (copies, deletes, etc.) is a queued, animated notice:
// each message types in character-by-character, holds, then types out again —
// the same reveal/dissolve as the Tavern subtitle. Messages queue rather than
// replace, so a burst plays one after another; when the queue drains, the slot
// returns to its default (e.g. "F1 help").
type toastPhase int

const (
	toastIdle toastPhase = iota
	toastEnter
	toastHold
	toastExit
)

const (
	toastFrameDur    = 45 * time.Millisecond
	toastEnterFrames = 12 // type-in duration
	toastHoldFrames  = 60 // full-text hold (~2.7s)
	toastExitFrames  = 10 // type-out duration
)

// toastTickMsg drives one animation frame; gen guards against a stale tick from
// a loop that already ended.
type toastTickMsg struct{ gen int }

// showClipboardToast enqueues the default "copied to clipboard" notice.
func (m *Model) showClipboardToast() tea.Cmd {
	return m.showClipboardToastText("")
}

// showClipboardToastText enqueues text as a status-line notice. If nothing is
// currently animating it starts right away; otherwise it plays after the ones
// ahead of it. Empty text falls back to "copied to clipboard".
func (m *Model) showClipboardToastText(text string) tea.Cmd {
	if text == "" {
		text = "copied to clipboard"
	}
	// Drop duplicates: if this exact notice is already showing or waiting in the
	// queue, don't re-add it. A runaway trigger (e.g. a drag-select firing the
	// same copy thousands of times) would otherwise pile up identical toasts that
	// then drain one at a time. The same message shows again fine once the current
	// one finishes and the slot returns to idle.
	if m.toastPhase != toastIdle && m.toastText == text {
		return nil
	}
	for _, q := range m.toastQueue {
		if q == text {
			return nil
		}
	}
	m.toastQueue = append(m.toastQueue, text)
	if m.toastPhase == toastIdle {
		return m.startNextToast()
	}
	return nil // already animating; the running loop will pick this up
}

// startNextToast pops the next queued message and begins its type-in, or goes
// idle (returning the slot to its default) when the queue is empty.
func (m *Model) startNextToast() tea.Cmd {
	if len(m.toastQueue) == 0 {
		m.toastPhase = toastIdle
		m.toastText = ""
		return nil
	}
	m.toastText = m.toastQueue[0]
	m.toastQueue = m.toastQueue[1:]
	m.toastPhase = toastEnter
	m.toastFrame = 0
	m.toastGen++
	return m.toastTick()
}

func (m *Model) toastTick() tea.Cmd {
	gen := m.toastGen
	return tea.Tick(toastFrameDur, func(time.Time) tea.Msg { return toastTickMsg{gen: gen} })
}

// advanceToast steps the animation one frame and schedules the next tick (or
// starts the next queued message / goes idle at the end of the type-out).
func (m *Model) advanceToast(gen int) tea.Cmd {
	if gen != m.toastGen || m.toastPhase == toastIdle {
		return nil
	}
	m.toastFrame++
	switch m.toastPhase {
	case toastEnter:
		if m.toastFrame >= toastEnterFrames {
			m.toastPhase, m.toastFrame = toastHold, 0
		}
	case toastHold:
		if m.toastFrame >= toastHoldFrames {
			m.toastPhase, m.toastFrame = toastExit, 0
		}
	case toastExit:
		if m.toastFrame >= toastExitFrames {
			return m.startNextToast()
		}
	}
	return m.toastTick()
}

// toastActive reports whether a notice is currently showing (any non-idle phase).
func (m *Model) toastActive() bool { return m.toastPhase != toastIdle }

// renderToast is the status-line notice with its type-in / type-out reveal — a
// bullet plus however much of the text the current frame exposes.
func (m *Model) renderToast() string {
	if m.toastPhase == toastIdle {
		return ""
	}
	full := []rune(m.toastText)
	shown := full
	switch m.toastPhase {
	case toastEnter:
		shown = full[:clampInt(m.toastFrame*len(full)/toastEnterFrames, 0, len(full))]
	case toastExit:
		keep := len(full) - m.toastFrame*len(full)/toastExitFrames
		shown = full[:clampInt(keep, 0, len(full))]
	}
	bullet := lipgloss.NewStyle().Bold(true).Foreground(ui.ColorHeading).Render("●")
	return bullet + " " + ui.StyleMuted.Render(string(shown))
}
