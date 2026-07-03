package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

const animFPS = 60 * time.Millisecond

type introTickMsg struct{}

func introTick() tea.Cmd {
	return tea.Tick(animFPS, func(time.Time) tea.Msg { return introTickMsg{} })
}

// advanceIntro steps the startup shine/typewriter animation (see
// ui.RenderLogoIntro) — View() renders it in place, in the logo's normal
// spot, so nothing about the rest of the screen changes when it finishes.
func (m *Model) advanceIntro() tea.Cmd {
	if m.introDone {
		return nil
	}
	m.introFrame++
	if m.introFrame >= ui.IntroTotalFrames(m.subtitle) {
		m.introDone = true
		return nil
	}
	return introTick()
}

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

const clipboardToastDuration = 1500 * time.Millisecond

// clipboardToastExpireMsg carries the generation it was scheduled for, so a
// stale timer from an earlier copy can't cut off a newer one's indicator —
// each fresh copy just keeps it showing a bit longer.
type clipboardToastExpireMsg struct{ gen int }

// showClipboardToast briefly swaps the footer (or the focused view's
// header) for a "copied to clipboard" indicator — see copySelection.
func (m *Model) showClipboardToast() tea.Cmd {
	m.clipboardToastGen++
	gen := m.clipboardToastGen
	m.clipboardToastActive = true
	return tea.Tick(clipboardToastDuration, func(time.Time) tea.Msg { return clipboardToastExpireMsg{gen: gen} })
}

func (m *Model) clearClipboardToastIfCurrent(gen int) {
	if gen == m.clipboardToastGen {
		m.clipboardToastActive = false
	}
}
