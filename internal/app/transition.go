package app

import (
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The environment-change animation. Moving between the Tavern and Afield (or
// filtering, or launching) plays the same beat in both directions: the current
// list burns away bottom-up, character by character (right-to-left), the block
// collapsing toward center as lines vanish; the subtitle types out and the
// header word mutes letter by letter; a brief pause; then the new subtitle
// types in, the new header word lights up, and the new list reveals line by
// line, the block growing back out. Because it re-centers every frame, the
// final frame already sits where the resting view does — no end jump.

type transPhase int

const (
	transNone transPhase = iota
	transDissolve
	transPause
	transReveal
)

// Per-frame tuning. Filter changes run faster than mode switches (transFast).
const (
	dissolveStepSlow = 3 // columns burned per frame
	dissolveStepFast = 12
	revealEverySlow  = 2 // frames per revealed line
	revealEveryFast  = 1
	pauseFramesSlow  = 5
	pauseFramesFast  = 1
	burnTrail        = 3 // trailing columns dimmed (burning) before they vanish
	modeWordLen      = 6 // "TAVERN" / "AFIELD"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

type transTickMsg struct{}

func transTick(fast bool) tea.Cmd {
	d := 38 * time.Millisecond
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

// currentRowLines renders the visible rows to styled strings (no cursor/hints).
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

// totalOldChars is the sum of the captured rows' widths — how many columns the
// dissolve has to burn through before it's done.
func (m *Model) totalOldChars() int {
	n := 0
	for _, l := range m.transOld {
		n += len([]rune(stripANSI(l)))
	}
	return n
}

func (m *Model) dissolveFraction() float64 {
	total := m.totalOldChars()
	if total == 0 {
		return 1
	}
	f := float64(m.transFrame*m.dissolveStep()) / float64(total)
	if f > 1 {
		f = 1
	}
	return f
}

func (m *Model) revealFraction() float64 {
	n := len(m.currentRowLines())
	if n == 0 {
		return 1
	}
	f := float64(m.transFrame/m.revealEvery()) / float64(n)
	if f > 1 {
		f = 1
	}
	return f
}

func (m *Model) advanceTransition() tea.Cmd {
	m.transFrame++
	switch m.transPhase {
	case transDissolve:
		if m.transFrame*m.dissolveStep() >= m.totalOldChars() {
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

// dissolveLines burns the captured rows away bottom-up: `erased` columns are
// consumed from the last line's right edge, carrying up to the line above once
// a line is spent. Fully-burned trailing lines drop out entirely, so the block
// shrinks line by line while the active line burns character by character.
func (m *Model) dissolveLines() []string {
	erased := m.transFrame * m.dissolveStep()
	lines := make([]string, len(m.transOld))
	copy(lines, m.transOld)
	for i := len(lines) - 1; i >= 0 && erased > 0; i-- {
		runes := []rune(stripANSI(lines[i]))
		if erased >= len(runes) {
			erased -= len(runes)
			lines[i] = ""
			continue
		}
		keep := len(runes) - erased
		head := string(runes[:max0(keep-burnTrail)])
		tail := string(runes[max0(keep-burnTrail):keep])
		lines[i] = head + ui.StyleMuted.Render(tail)
		erased = 0
	}
	end := len(lines)
	for end > 0 && lines[end-1] == "" {
		end--
	}
	return lines[:end]
}

// revealLines fills the new rows back in top-down, one every revealEvery frames.
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

// transitionRows is the row lines to show this frame.
func (m *Model) transitionRows() []string {
	switch m.transPhase {
	case transDissolve:
		return m.dissolveLines()
	case transReveal:
		return m.revealLines()
	}
	return nil // pause
}

// transitionSubtitle types the subtitle out (dissolve) and back in (reveal).
// Filter changes leave it untouched.
func (m *Model) transitionSubtitle() string {
	if m.transFast {
		return m.subtitle
	}
	switch m.transPhase {
	case transDissolve:
		r := []rune(m.transOldSub)
		keep := len(r) - int(m.dissolveFraction()*float64(len(r)))
		return string(r[:max0(keep)])
	case transPause:
		return ""
	case transReveal:
		r := []rune(m.subtitle)
		n := int(m.revealFraction() * float64(len(r)))
		if n > len(r) {
			n = len(r)
		}
		return string(r[:n])
	}
	return m.subtitle
}

func (m *Model) transitioning() bool { return m.transPhase != transNone }

// modeSpan is a TAVERN/AFIELD header label's clickable extent.
type modeSpan struct {
	x0, x1 int
	afield bool
}

func allBools(n int, v bool) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// litFromLeft/litFromRight build per-letter "is bright" masks: k letters set
// from the left or right end, the rest the opposite.
func litFromLeft(n, k int, set bool) []bool {
	out := allBools(n, !set)
	for i := 0; i < k && i < n; i++ {
		out[i] = set
	}
	return out
}
func litFromRight(n, k int, set bool) []bool {
	out := allBools(n, !set)
	for i := 0; i < k && i < n; i++ {
		out[n-1-i] = set
	}
	return out
}

// animatedModeLetters returns the per-letter bright masks for TAVERN and
// AFIELD this frame. TAVERN always animates left-to-right, AFIELD right-to-
// left. During the dissolve the outgoing word mutes; during the reveal the
// incoming (now-active) word lights up; between, both are muted.
func (m *Model) animatedModeLetters() (tav, afi []bool) {
	n := modeWordLen
	toAfield := m.afield // destination (state already switched)
	switch m.transPhase {
	case transDissolve:
		k := int(m.dissolveFraction() * float64(n))
		if toAfield { // leaving Tavern: TAVERN mutes L→R
			return litFromLeft(n, k, false), allBools(n, false)
		}
		return allBools(n, false), litFromRight(n, k, false) // AFIELD mutes R→L
	case transReveal:
		k := int(m.revealFraction() * float64(n))
		if toAfield { // arriving Afield: AFIELD lights R→L
			return allBools(n, false), litFromRight(n, k, true)
		}
		return litFromLeft(n, k, true), allBools(n, false) // TAVERN lights L→R
	case transPause:
		return allBools(n, false), allBools(n, false)
	}
	return allBools(n, !toAfield), allBools(n, toAfield)
}

// renderHeader is the two banner lines: the TAVERN/AFIELD toggle and the
// flavor subtitle (used by the resting view).
func (m *Model) renderHeader(width int) []string {
	return []string{
		m.renderModeToggle(width),
		ui.CenterText(ui.StyleMuted.Render(m.subtitle), width),
	}
}

// renderModeToggle draws the resting header: the active mode bright, the other
// muted.
func (m *Model) renderModeToggle(width int) string {
	return m.renderModeLine(width, allBools(modeWordLen, !m.afield), allBools(modeWordLen, m.afield))
}

// renderModeLine draws "TAVERN   AFIELD" with per-letter brightness, centering
// the two words as a unit exactly where QUESTS sat (the trailing ⌃G hint is
// appended without shifting that center). Records modeSpans for clicks.
func (m *Model) renderModeLine(width int, litTav, litAfi []bool) string {
	const tav, afi, gap = "TAVERN", "AFIELD", "   "
	coreW := len([]rune(tav)) + len([]rune(gap)) + len([]rune(afi))
	startX := m.leftMargin + (width-coreW)/2
	if startX < m.leftMargin {
		startX = m.leftMargin
	}
	tw, aw := len([]rune(tav)), len([]rune(afi))
	m.modeSpans = []modeSpan{
		{x0: startX, x1: startX + tw, afield: false},
		{x0: startX + tw + len([]rune(gap)), x1: startX + tw + len([]rune(gap)) + aw, afield: true},
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", startX))
	b.WriteString(styledWord(tav, litTav))
	b.WriteString(gap)
	b.WriteString(styledWord(afi, litAfi))
	if !m.hideHoverTips {
		b.WriteString(ui.StyleMuted.Render("  ⌃G"))
	}
	return b.String()
}

func styledWord(word string, lit []bool) string {
	var b strings.Builder
	for i, r := range word {
		st := ui.StyleMuted
		if i < len(lit) && lit[i] {
			st = ui.StyleTitle
		}
		b.WriteString(st.Render(string(r)))
	}
	return b.String()
}

// renderTransitionView draws one animation frame, re-centered on the CURRENT
// row count each frame so the block collapses to the header (dissolve) and
// grows back out (reveal) with no end jump.
func (m *Model) renderTransitionView() string {
	width := m.contentWidth()
	m.leftMargin = (m.width - width) / 2
	if m.leftMargin < 0 {
		m.leftMargin = 0
	}
	margin := strings.Repeat(" ", m.leftMargin)

	litTav, litAfi := m.animatedModeLetters()
	header := []string{
		m.renderModeLine(width, litTav, litAfi),
		ui.CenterText(ui.StyleMuted.Render(m.transitionSubtitle()), width),
	}
	logoHeight := len(header) + 3 // blank, filter line, blank (matches resting view)

	rowLines := m.transitionRows()
	rowCount := len(rowLines)

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
	m.modeToggleRow = topPad

	clip := lipgloss.NewStyle().MaxWidth(m.width)
	var b strings.Builder
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}
	for _, line := range header {
		b.WriteString(clip.Render(margin+line) + "\n")
	}
	b.WriteString("\n\n\n") // blank / reserved filter line / blank
	for _, line := range rowLines {
		b.WriteString(clip.Render(margin+line) + "\n")
	}
	return b.String()
}
