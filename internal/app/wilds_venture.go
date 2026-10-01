package app

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The Wilds focus loop: from Camp (the full taken-up agenda) you venture out
// with ONE quest into the Wilds — just that quest and its objectives, nothing
// else, with a count-up session timer running. Finishing the quest (or its
// last objective) returns you to Camp with a flourish; Esc "makes camp" back
// with no penalty. Every trip is logged (store.WildsSessions), feeding the
// gentle "focused today" stat.

// ventureTickMsg drives the once-a-second refresh of the session timer.
type ventureTickMsg struct{ gen int }

func ventureTick(gen int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return ventureTickMsg{gen: gen} })
}

// venturing reports whether a single quest is currently ventured into the Wilds.
func (m *Model) venturing() bool { return m.venturedID != "" }

// ventureRows is the distraction-free Wilds body: the ventured quest and its
// pending objectives, and nothing else.
func (m *Model) ventureRows() []ui.Row {
	q := m.findQuest(m.venturedID)
	if q == nil {
		return nil
	}
	rows := []ui.Row{{Kind: ui.RowQuest, ProjectID: q.ProjectID, QuestID: q.ID, ShowProjectTag: true}}
	return append(rows, wildsObjectiveRows(*q)...)
}

// venture heads out of Camp with a single quest — the focus ritual. The header
// stays CAMP-lit (a kindFilter transition keeps the mode letters static); the
// body dissolves down to just this quest, and the session timer starts.
func (m *Model) venture(questID string) tea.Cmd {
	if m.findQuest(questID) == nil {
		return nil
	}
	m.commitEdit()
	old := m.currentRowLines()
	m.venturedID = questID
	m.ventureStart = time.Now()
	m.ventureTickGen++
	m.editor = nil
	m.scrollOffset = 0
	m.subtitle = ui.RandomWildsGreeting() // brief arrival flavor before the timer settles in
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	} else {
		m.cursor = cursorTarget{}
	}
	return tea.Batch(
		m.beginTransition(old, kindVenture),
		m.playSound(sndEnterWilds),
		ventureTick(m.ventureTickGen),
	)
}

// makeCamp returns from the Wilds to Camp, logging the just-finished session
// first. completed drives the flourish: a finished quest gets the done-sound
// and a themed line ("You return to camp — …"); a plain bail gets a quiet
// return. Bailing is never penalized — the time still logs.
func (m *Model) makeCamp(completed bool) tea.Cmd {
	if !m.venturing() {
		return nil
	}
	q := m.findQuest(m.venturedID)
	dur := m.logVentureSession(completed)

	m.commitEdit()
	old := m.currentRowLines()
	m.endVenture()
	m.editor = nil
	m.scrollOffset = 0

	snd := sndEnterTavern
	if completed && q != nil {
		m.subtitle = fmt.Sprintf("You return to camp — %s done, %s in the wilds.", questShortName(q.Title), humanizeVentureDur(dur))
		snd = sndQuestDone
	} else {
		m.subtitle = ui.RandomCampGreeting()
	}
	if rows := m.visibleRows(); len(rows) > 0 {
		m.setCursor(rows[0])
	} else {
		m.cursor = cursorTarget{}
	}
	return tea.Batch(m.beginTransition(old, kindVenture), m.playSound(snd))
}

// completeVenture finishes the ventured quest: mark it done, seal it with a
// sparkle where it sits, then dissolve back to Camp with the flourish.
func (m *Model) completeVenture() tea.Cmd {
	q := m.findQuest(m.venturedID)
	if q == nil {
		return m.makeCamp(false)
	}
	if q.Status != model.StatusDone {
		now := time.Now()
		if !recycleRite(q) { // a rite rolls to its next muster instead of finishing
			q.Status = model.StatusDone
			q.CompletedAt = &now
		}
		q.UpdatedAt = now
		m.save()
	}
	if m.cursorScreenY >= 0 {
		m.spawnSparkleRing(m.cursorScreenX+questGlyphCol, m.cursorScreenY, 12)
	}
	return tea.Batch(m.makeCamp(true), m.maybeStartOverlayTick())
}

// logVentureSession appends the current venture as a session record and returns
// its duration. Safe to call only while venturing.
func (m *Model) logVentureSession(completed bool) time.Duration {
	if m.venturedID == "" {
		return 0
	}
	s := model.WildsSession{QuestID: m.venturedID, Start: m.ventureStart, End: time.Now(), Completed: completed}
	m.store.WildsSessions = append(m.store.WildsSessions, s)
	m.save()
	return s.Duration()
}

// endVenture clears the ventured quest and invalidates any pending timer tick.
func (m *Model) endVenture() {
	m.venturedID = ""
	m.ventureTickGen++ // any tick already in flight is now stale
}

// ventureTimerLabel is the live count-up shown in the header while venturing.
func (m *Model) ventureTimerLabel() string {
	return "⏱ " + formatMMSS(time.Since(m.ventureStart))
}

// ventureHeaderLine is the Wilds header: "WILDS" centered, the session timer
// pinned to the right. There's no TAVERN/CAMP toggle here (you Esc / Ctrl+G to
// leave), so it clears the toggle's click targets.
func (m *Model) ventureHeaderLine(width int) string {
	m.modeSpans = nil
	m.tavernHelpWidth = 0
	label := ui.StyleTitle.Render("WILDS")
	timer := ui.StyleMuted.Render(m.ventureTimerLabel())
	pad := (width - lipgloss.Width(label)) / 2
	if pad < 0 {
		pad = 0
	}
	line := strings.Repeat(" ", pad) + label
	slack := width - lipgloss.Width(line) - lipgloss.Width(timer)
	if slack < 1 {
		slack = 1
	}
	return line + strings.Repeat(" ", slack) + timer
}

// formatMMSS renders a duration as MM:SS (H:MM:SS past an hour) for the timer.
func formatMMSS(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	h, mnt, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, mnt, s)
	}
	return fmt.Sprintf("%02d:%02d", mnt, s)
}

// humanizeVentureDur renders a duration compactly for prose ("45s", "24m",
// "1h 5m").
func humanizeVentureDur(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	h, mnt := int(d.Hours()), int(d.Minutes())%60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, mnt)
	}
	return fmt.Sprintf("%dm", mnt)
}

// questShortName is a quest title trimmed for a one-line flourish.
func questShortName(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "the quest"
	}
	r := []rune(title)
	if len(r) > 40 {
		return string(r[:39]) + "…"
	}
	return title
}
