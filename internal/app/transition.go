package app

import (
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The environment-change animation. When you move between the Tavern and
// Afield (or filter, or launch the app), the current list burns away
// right-to-left character by character, pauses a beat, then the new view
// reveals line by line — as if walking from one place to another.

type transPhase int

const (
	transNone transPhase = iota
	transDissolve
	transPause
	transReveal
)

// Tuning per frame. Filter changes run faster than mode switches (transFast).
const (
	dissolveStepSlow = 3 // columns burned off each line per frame
	dissolveStepFast = 10
	revealEverySlow  = 2 // frames per revealed line
	revealEveryFast  = 1
	pauseFramesSlow  = 6
	pauseFramesFast  = 2
	burnTrail        = 3 // trailing columns shown muted (burning) before they vanish
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

type transTickMsg struct{}

func transTick(fast bool) tea.Cmd {
	d := 40 * time.Millisecond
	if fast {
		d = 16 * time.Millisecond
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return transTickMsg{} })
}

// beginTransition starts the dissolve from oldLines (captured before the state
// change) into whatever the new state now renders. No-op when animations are
// off. Pass fast=true for filter changes.
func (m *Model) beginTransition(oldLines []string, fast bool) tea.Cmd {
	if !m.animate {
		m.transPhase = transNone
		return nil
	}
	m.transOld = oldLines
	m.transFast = fast
	m.transFrame = 0
	m.transPhase = transDissolve
	m.scrollOffset = 0
	return transTick(fast)
}

// currentRowLines renders the visible rows to plain (styled) strings with no
// cursor/hints — the snapshot the dissolve burns away.
func (m *Model) currentRowLines() []string {
	cw := m.contentWidth()
	rows := m.visibleRows()
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		line, _ := ui.RenderRow(r, m.store, "", false, cw, "")
		out = append(out, line)
	}
	return out
}

func (m *Model) dissolveStep() int {
	if m.transFast {
		return dissolveStepFast
	}
	return dissolveStepSlow
}

func (m *Model) revealEvery() int {
	if m.transFast {
		return revealEveryFast
	}
	return revealEverySlow
}

func (m *Model) pauseFrames() int {
	if m.transFast {
		return pauseFramesFast
	}
	return pauseFramesSlow
}

func (m *Model) maxOldWidth() int {
	w := 0
	for _, l := range m.transOld {
		if n := lipgloss.Width(l); n > w {
			w = n
		}
	}
	return w
}

func (m *Model) advanceTransition() tea.Cmd {
	m.transFrame++
	switch m.transPhase {
	case transDissolve:
		if m.transFrame*m.dissolveStep() >= m.maxOldWidth() {
			m.transPhase = transPause
			m.transFrame = 0
		}
	case transPause:
		if m.transFrame >= m.pauseFrames() {
			m.transPhase = transReveal
			m.transFrame = 0
		}
	case transReveal:
		if m.transFrame/m.revealEvery() >= len(m.currentRowLines()) {
			m.transPhase = transNone
			m.transOld = nil
			return nil
		}
	}
	return transTick(m.transFast)
}

// dissolveLines returns the old rows with their trailing `burned` columns
// stripped (right-to-left), the last few surviving columns dimmed as if
// burning out.
func (m *Model) dissolveLines() []string {
	burned := m.transFrame * m.dissolveStep()
	out := make([]string, 0, len(m.transOld))
	for _, styled := range m.transOld {
		runes := []rune(stripANSI(styled))
		keep := len(runes) - burned
		if keep <= 0 {
			out = append(out, "")
			continue
		}
		head := string(runes[:max0(keep-burnTrail)])
		tail := string(runes[max0(keep-burnTrail):keep])
		out = append(out, head+ui.StyleMuted.Render(tail))
	}
	return out
}

// revealLines returns the first N new rows (N grows each frame) for the
// reveal, so the list fills back in line by line.
func (m *Model) revealLines() []string {
	all := m.currentRowLines()
	n := m.transFrame / m.revealEvery()
	if n > len(all) {
		n = len(all)
	}
	return all[:n]
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// transitionRows returns the row lines to display for the current animation
// phase, and whether the subtitle should show yet (it appears once the old
// view has fully burned away).
func (m *Model) transitionRows() (lines []string, showSubtitle bool) {
	switch m.transPhase {
	case transDissolve:
		return m.dissolveLines(), false
	case transPause:
		return nil, false
	case transReveal:
		return m.revealLines(), true
	}
	return nil, true
}

// transitioning reports whether an animation is mid-flight.
func (m *Model) transitioning() bool { return m.transPhase != transNone }

// modeSpan is a TAVERN/AFIELD header label's clickable extent.
type modeSpan struct {
	x0, x1 int
	afield bool // which mode clicking it selects
}

// renderHeader is the two banner lines: the TAVERN/AFIELD toggle and the
// flavor subtitle.
func (m *Model) renderHeader(width int) []string {
	return []string{
		m.renderModeToggle(width),
		ui.CenterText(ui.StyleMuted.Render(m.subtitle), width),
	}
}

// renderModeToggle draws "TAVERN   AFIELD" centered, the active side bright
// and the other muted (like the quick chips), with a muted Ctrl+G hint unless
// hover tips are hidden. Records modeSpans for click hit-testing.
func (m *Model) renderModeToggle(width int) string {
	m.modeSpans = nil
	const tav, afi, gap = "TAVERN", "AFIELD", "   "
	hintPlain := ""
	if !m.hideHoverTips {
		hintPlain = "  ⌃G"
	}
	plainWidth := len([]rune(tav)) + len([]rune(gap)) + len([]rune(afi)) + len([]rune(hintPlain))
	startX := m.leftMargin + (width-plainWidth)/2
	if startX < m.leftMargin {
		startX = m.leftMargin
	}
	tavStyle, afiStyle := ui.StyleTitle, ui.StyleMuted
	if m.afield {
		tavStyle, afiStyle = ui.StyleMuted, ui.StyleTitle
	}
	tw, aw := len([]rune(tav)), len([]rune(afi))
	m.modeSpans = []modeSpan{
		{x0: startX, x1: startX + tw, afield: false},
		{x0: startX + tw + len([]rune(gap)), x1: startX + tw + len([]rune(gap)) + aw, afield: true},
	}
	line := tavStyle.Render(tav) + gap + afiStyle.Render(afi)
	if hintPlain != "" {
		line += ui.StyleMuted.Render(hintPlain)
	}
	return strings.Repeat(" ", startX) + line
}

// renderTransitionView draws a single animation frame: the header (with the
// subtitle hidden until the reveal), and the dissolving/revealing rows,
// vertically centered like the resting view so it doesn't jump when it ends.
func (m *Model) renderTransitionView() string {
	width := m.contentWidth()
	m.leftMargin = (m.width - width) / 2
	if m.leftMargin < 0 {
		m.leftMargin = 0
	}
	margin := strings.Repeat(" ", m.leftMargin)

	rowLines, showSub := m.transitionRows()
	sub := ""
	if showSub {
		sub = m.subtitle
	}
	header := []string{
		m.renderModeToggle(width),
		ui.CenterText(ui.StyleMuted.Render(sub), width),
	}
	logoHeight := len(header) + 3 // blank, filter line, blank (matches the resting view)

	// Hold the vertical position stable using the larger of the old/new row
	// counts, so lines fill in without the block drifting.
	rowCount := len(m.transOld)
	if n := len(m.currentRowLines()); n > rowCount {
		rowCount = n
	}

	availableHeight := m.height - 1 // one footer line
	if availableHeight < 1 {
		availableHeight = 1
	}
	vpad := viewVPad
	if maxPad := availableHeight / 4; vpad > maxPad {
		vpad = maxPad
	}
	if vpad < 0 {
		vpad = 0
	}
	innerHeight := availableHeight - 2*vpad
	if innerHeight < 1 {
		innerHeight = 1
	}
	blockHeight := logoHeight + rowCount
	topPad := vpad + (innerHeight-blockHeight)/2
	if topPad < 0 {
		topPad = 0
	}

	clip := lipgloss.NewStyle().MaxWidth(m.width)
	var b strings.Builder
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}
	for _, line := range header {
		b.WriteString(clip.Render(margin+line) + "\n")
	}
	b.WriteString("\n\n\n") // blank / (reserved filter) / blank
	for i := 0; i < rowCount; i++ {
		if i < len(rowLines) {
			b.WriteString(clip.Render(margin+rowLines[i]) + "\n")
		} else {
			b.WriteString("\n")
		}
	}
	return b.String()
}
