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
// list burns away bottom-up, character by character, the block collapsing
// toward center as lines vanish; the subtitle types out and the header word
// mutes; a brief pause; then the new subtitle types in, the header word lights
// up, and the new list reveals line by line, growing back out. It re-centers
// every frame, so the final frame already sits where the resting view does.
//
// Timing is FIXED in frames (not content-dependent), so a switch is always the
// same length regardless of list size. Each element has its own pace: the list
// is fastest, the header a touch slower, the subtitle slowest.

type transPhase int

const (
	transNone transPhase = iota
	transDissolve
	transPause
	transReveal
)

const (
	listFramesSlow  = 9
	listFramesFast  = 5
	headerFramesN   = 10
	subFramesN      = 15 // subtitle typewriter (a touch slower)
	leadBeat        = 4  // beat after the subtitle finishes before the list starts
	pauseFramesSlow = 3
	pauseFramesFast = 1
	burnTrail       = 3 // trailing columns dimmed (burning) before they vanish
	modeWordLen     = 6 // "TAVERN" / "AFIELD"
)

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// transKind distinguishes the three triggers, which animate differently:
// startup reveals only (no prior view/mode), a mode switch does the full
// dissolve→reveal with the header sweep, and a filter change is a quick
// list-only re-form with a static header/subtitle.
type transKind int

const (
	kindMode transKind = iota
	kindFilter
	kindStartup
)

type transTickMsg struct{ gen int }

func transTick(fast bool, gen int) tea.Cmd {
	d := 38 * time.Millisecond
	if fast {
		d = 16 * time.Millisecond
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return transTickMsg{gen: gen} })
}

func (m *Model) beginTransition(oldLines []string, kind transKind) tea.Cmd {
	if !m.animate {
		m.transPhase = transNone
		return nil
	}
	m.transOld = oldLines
	m.transKind = kind
	m.transFast = kind == kindFilter
	m.transFrame = 0
	// Startup has no previous view to burn away — reveal straight in.
	if kind == kindStartup {
		m.transPhase = transReveal
	} else {
		m.transPhase = transDissolve
	}
	m.scrollOffset = 0
	// New generation: any in-flight ticker from a previous transition (e.g.
	// an interrupted switch or rapid filter changes) is now stale and ignored,
	// so only one ticker chain ever advances the frame — no 2x speed-up.
	m.transGen++
	return transTick(m.transFast, m.transGen)
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

// --- fixed per-element timings -------------------------------------------

func (m *Model) listFrames() int {
	if m.transFast {
		return listFramesFast
	}
	return listFramesSlow
}
func (m *Model) headerFrames() int { return headerFramesN }
func (m *Model) subFrames() int    { return subFramesN }

func (m *Model) pauseFrames() int {
	if m.transFast {
		return pauseFramesFast
	}
	return pauseFramesSlow
}

// listLead is how many frames the header/subtitle get before the list starts
// (revealing) — so "TAVERN lights up, subtitle loads, then the list loads".
// Filter changes have no header/subtitle, so the list starts immediately.
func (m *Model) listLead() int {
	if m.transKind == kindFilter {
		return 0
	}
	// Let the subtitle fully type in, then a short beat, before the list.
	return m.subFrames() + leadBeat
}

// dissolvePhaseFrames / revealPhaseFrames: how long each half runs. The
// dissolve burns everything concurrently; the reveal staggers the list after
// the header/subtitle.
func (m *Model) dissolvePhaseFrames() int {
	if m.transFast {
		return m.listFrames()
	}
	return m.subFrames()
}

func (m *Model) revealPhaseFrames() int {
	if m.transFast {
		return m.listFrames()
	}
	lead := m.listLead() + m.listFrames()
	if m.subFrames() > lead {
		return m.subFrames()
	}
	return lead
}

func frac(frame, frames int) float64 {
	if frames <= 0 {
		return 1
	}
	f := float64(frame) / float64(frames)
	if f > 1 {
		f = 1
	}
	return f
}

func (m *Model) listFraction() float64   { return frac(m.transFrame, m.listFrames()) }
func (m *Model) headerFraction() float64 { return frac(m.transFrame, m.headerFrames()) }
func (m *Model) subFraction() float64    { return frac(m.transFrame, m.subFrames()) }

func (m *Model) totalOldChars() int {
	n := 0
	for _, l := range m.transOld {
		n += len([]rune(stripANSI(l)))
	}
	return n
}

func (m *Model) advanceTransition() tea.Cmd {
	m.transFrame++
	switch m.transPhase {
	case transDissolve:
		if m.transFrame >= m.dissolvePhaseFrames() {
			m.transPhase = transPause
			m.transFrame = 0
		}
	case transPause:
		if m.transFrame >= m.pauseFrames() {
			m.transPhase = transReveal
			m.transFrame = 0
		}
	case transReveal:
		if m.transFrame >= m.revealPhaseFrames() {
			m.transPhase = transNone
			m.transOld = nil
			return nil
		}
	}
	return transTick(m.transFast, m.transGen)
}

// dissolveLines burns the captured rows away bottom-up over listFrames frames:
// a fixed fraction of the total characters is consumed from the last line's
// right edge, carrying up as lines are spent. Fully-burned trailing lines drop
// out, so the block shrinks line by line while the active line burns per char.
func (m *Model) dissolveLines() []string {
	erased := int(m.listFraction() * float64(m.totalOldChars()))
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

// revealLines fills the new rows back in top-down, starting only after the
// listLead frames (so the header/subtitle come in first).
func (m *Model) revealLines() []string {
	all := m.currentRowLines()
	f := frac(max0(m.transFrame-m.listLead()), m.listFrames())
	n := int(f * float64(len(all)))
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

func (m *Model) transitionRows() []string {
	switch m.transPhase {
	case transDissolve:
		return m.dissolveLines()
	case transReveal:
		return m.revealLines()
	}
	return nil // pause
}

// transitionSubtitle types the subtitle out (dissolve) and in (reveal) over
// subFrames. Filter changes leave it untouched.
func (m *Model) transitionSubtitle() string {
	if m.transFast {
		return m.subtitle
	}
	switch m.transPhase {
	case transDissolve:
		r := []rune(m.transOldSub)
		keep := len(r) - int(m.subFraction()*float64(len(r)))
		return string(r[:max0(keep)])
	case transPause:
		return ""
	case transReveal:
		r := []rune(m.subtitle)
		n := int(m.subFraction() * float64(len(r)))
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

// animatedModeLetters returns the per-letter brightness for TAVERN and AFIELD.
// The sweep flows in the switch direction so one word pours into the other: to
// Afield it runs left-to-right (TAVERN mutes L→R, then AFIELD lights L→R); to
// Tavern it runs right-to-left.
func (m *Model) animatedModeLetters() (tav, afi []bool) {
	n := modeWordLen
	toAfield := m.afield
	if m.transKind == kindFilter {
		// Filter changes don't switch mode — keep the header static.
		return allBools(n, !toAfield), allBools(n, toAfield)
	}
	if m.transKind == kindStartup {
		// No previous mode: just light TAVERN in, left-to-right; AFIELD stays
		// muted. (Startup is reveal-only.)
		k := int(m.headerFraction() * float64(n))
		return litFromLeft(n, k, true), allBools(n, false)
	}
	switch m.transPhase {
	case transDissolve:
		k := int(m.headerFraction() * float64(n))
		if toAfield {
			return litFromLeft(n, k, false), allBools(n, false)
		}
		return allBools(n, false), litFromRight(n, k, false)
	case transReveal:
		k := int(m.headerFraction() * float64(n))
		if toAfield {
			return allBools(n, false), litFromLeft(n, k, true)
		}
		return litFromRight(n, k, true), allBools(n, false)
	case transPause:
		return allBools(n, false), allBools(n, false)
	}
	return allBools(n, !toAfield), allBools(n, toAfield)
}

// renderHeader is the two banner lines for the resting view.
func (m *Model) renderHeader(width int) []string {
	return []string{
		m.renderModeToggle(width),
		ui.CenterText(ui.StyleMuted.Render(m.subtitle), width),
	}
}

func (m *Model) renderModeToggle(width int) string {
	return m.renderModeLine(width, allBools(modeWordLen, !m.afield), allBools(modeWordLen, m.afield))
}

// renderModeLine draws "TAVERN   AFIELD" with per-letter brightness, centering
// the two words as a unit exactly where QUESTS sat. Padding is RELATIVE (the
// caller prepends the left margin); modeSpans are absolute for click testing.
func (m *Model) renderModeLine(width int, litTav, litAfi []bool) string {
	const tav, afi, gap = "TAVERN", "AFIELD", "   "
	coreW := len([]rune(tav)) + len([]rune(gap)) + len([]rune(afi))
	pad := (width - coreW) / 2
	if pad < 0 {
		pad = 0
	}
	tw, aw := len([]rune(tav)), len([]rune(afi))
	absX := m.leftMargin + pad
	m.modeSpans = []modeSpan{
		{x0: absX, x1: absX + tw, afield: false},
		{x0: absX + tw + len([]rune(gap)), x1: absX + tw + len([]rune(gap)) + aw, afield: true},
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", pad))
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

// renderTransitionView draws one animation frame, re-centered on the current
// row count so the block collapses to the header and grows back out with no
// end jump. The filter line (Afield chips / open search bar) stays put.
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
	// Clamp the visible rows to the viewport, exactly like the resting view,
	// so a big list never spills past our boundaries mid-animation. When the
	// list is taller than the region, show what fits and mark "more below"
	// with the "···" fold.
	viewHeight := innerHeight - logoHeight
	if viewHeight < 1 {
		viewHeight = 1
	}
	shown := rowCount
	overflow := false
	if shown > viewHeight {
		shown = viewHeight - 1 // reserve a line for the bottom fold
		if shown < 0 {
			shown = 0
		}
		overflow = true
	}
	blockHeight := logoHeight + shown
	if overflow {
		blockHeight++
	}
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
	b.WriteString("\n")                                                  // blank after header
	b.WriteString(clip.Render(m.renderFilterLine(width, margin)) + "\n") // persistent chips / search bar
	b.WriteString("\n")                                                  // blank before rows
	m.modeToggleRow = topPad
	m.chipLineRow = topPad + len(header) + 1
	for _, line := range rowLines[:shown] {
		b.WriteString(clip.Render(margin+line) + "\n")
	}
	if overflow {
		b.WriteString(foldHint(margin, width) + "\n")
	}
	return b.String()
}
