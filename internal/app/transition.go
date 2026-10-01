package app

import (
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The environment-change animation. Moving between the Tavern and Wilds (or
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
	listFramesSlow  = 8
	listFramesFast  = 5
	headerFramesN   = 8
	subFramesN      = 10 // subtitle typewriter
	leadBeat        = 2  // short beat before the list starts revealing
	pauseFramesSlow = 2
	pauseFramesFast = 1
	burnTrail       = 3 // trailing columns dimmed (burning) before they vanish
)

// The two mode-toggle words (differ in length, so the letter animation uses
// each word's own length).
const (
	tavernLabel = "TAVERN"
	wildsLabel  = "CAMP"
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
	// kindVenture is the Camp⇄Wilds focus transition — slow like a mode switch,
	// but with a static header (the Wilds/Camp label swaps rather than the
	// TAVERN/CAMP letters animating).
	kindVenture
)

type transTickMsg struct{ gen int }

func transTick(fast bool, gen int) tea.Cmd {
	d := 40 * time.Millisecond
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
	m.transAbsolute = false // callers that capture margin-baked lines set this after
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
		if r.Kind == ui.RowQuestMeta {
			continue // meta sub-lines are app-level; skip in the cosmetic transition snapshot
		}
		line, _ := ui.RenderRow(r, m.store, "", false, false, cw, "")
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
	// A short beat before the list starts, so the header/subtitle lead slightly
	// but the body doesn't wait for the whole subtitle to finish typing.
	return leadBeat
}

// dissolvePhaseFrames / revealPhaseFrames: how long each half runs. The
// dissolve burns everything concurrently; the reveal staggers the list after
// the header/subtitle.
func (m *Model) dissolvePhaseFrames() int {
	return m.listFrames()
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

// revealProgress is how far the row reveal has run (0→1), after the header/
// subtitle lead. Shared by the sliding reveal and the Tavern open frame.
func (m *Model) revealProgress() float64 {
	return frac(max0(m.transFrame-m.listLead()), m.listFrames())
}

// lerpInt linearly interpolates between two integer positions, rounding.
func lerpInt(a, b int, t float64) int {
	return int(float64(a) + (float64(b)-float64(a))*t + 0.5)
}

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
// listLead frames (so the header/subtitle come in first). Each row slides in
// from a small right-offset that eases to zero, staggered so they cascade
// (matching the lab's "List reveal") rather than popping in whole.
func (m *Model) revealLines() []string {
	all := m.currentRowLines()
	if m.transAbsolute {
		all = m.currentBodyAbsolute() // the arriving view's margin-baked body (Tavern hall+pane, or Camp)
	}
	if len(all) == 0 {
		return nil
	}
	prog := m.revealProgress()
	const (
		slideCols   = 4
		slideWindow = 0.45 // fraction of the reveal each row spends sliding in
	)
	var out []string
	for i, line := range all {
		// (i+1)/(len+1) so even the FIRST row starts after a beat (nothing is
		// on screen at prog 0 — it too slides in), while the last still finishes
		// by prog=1.
		start := (float64(i+1) / float64(len(all)+1)) * (1 - slideWindow)
		if prog < start {
			break // not revealed yet
		}
		p := (prog - start) / slideWindow
		if p > 1 {
			p = 1
		}
		indent := int(float64(slideCols)*(1-p) + 0.5)
		out = append(out, strings.Repeat(" ", indent)+line)
	}
	return out
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

// modeSpan is a TAVERN/WILDS header label's clickable extent.
type modeSpan struct {
	x0, x1 int
	wilds  bool
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

// animatedModeLetters returns the per-letter brightness for TAVERN and WILDS.
// The sweep flows in the switch direction so one word pours into the other: to
// Wilds it runs left-to-right (TAVERN mutes L→R, then WILDS lights L→R); to
// Tavern it runs right-to-left.
func (m *Model) animatedModeLetters() (tav, wild []bool) {
	nt, nw := len([]rune(tavernLabel)), len([]rune(wildsLabel))
	toWilds := m.wilds
	if m.transKind == kindFilter || m.transKind == kindVenture {
		// Filter/venture changes don't switch the TAVERN/CAMP mode — keep the
		// letters static (venture swaps to its own WILDS header separately).
		return allBools(nt, !toWilds), allBools(nw, toWilds)
	}
	if m.transKind == kindStartup {
		// No previous mode: just light TAVERN in, left-to-right; WILDS stays
		// muted. (Startup is reveal-only.)
		k := int(m.headerFraction() * float64(nt))
		return litFromLeft(nt, k, true), allBools(nw, false)
	}
	switch m.transPhase {
	case transDissolve:
		if toWilds { // leaving Tavern: TAVERN mutes L→R
			return litFromLeft(nt, int(m.headerFraction()*float64(nt)), false), allBools(nw, false)
		}
		return allBools(nt, false), litFromRight(nw, int(m.headerFraction()*float64(nw)), false)
	case transReveal:
		if toWilds { // arriving Wilds: WILDS lights L→R
			return allBools(nt, false), litFromLeft(nw, int(m.headerFraction()*float64(nw)), true)
		}
		return litFromRight(nt, int(m.headerFraction()*float64(nt)), true), allBools(nw, false)
	case transPause:
		return allBools(nt, false), allBools(nw, false)
	}
	return allBools(nt, !toWilds), allBools(nw, toWilds)
}

// renderHeader is the banner shown above Camp/Wilds: the TAVERN/CAMP toggle and
// the greeting (the count-up timer replaces the greeting while venturing). The
// Tavern builds its own header in renderTavernView.
func (m *Model) renderHeader(width int) []string {
	if m.venturing() {
		// Deep in the Wilds the header is just "WILDS" + the session timer — no
		// TAVERN/CAMP toggle, for maximum focus.
		return []string{
			m.ventureHeaderLine(width),
			ui.CenterText(ui.StyleMuted.Render(m.subtitle), width),
		}
	}
	return []string{
		m.renderModeToggle(width),
		ui.CenterText(ui.StyleMuted.Render(m.subtitle), width),
	}
}

func (m *Model) renderModeToggle(width int) string {
	return m.renderModeLine(width, allBools(len([]rune(tavernLabel)), !m.wilds), allBools(len([]rune(wildsLabel)), m.wilds))
}

// renderModeLine draws "TAVERN   WILDS" with per-letter brightness, centering
// the two words as a unit exactly where QUESTS sat. Padding is RELATIVE (the
// caller prepends the left margin); modeSpans are absolute for click testing.
func (m *Model) renderModeLine(width int, litTav, litWild []bool) string {
	const gap = "   "
	tav, wild := tavernLabel, wildsLabel
	coreW := len([]rune(tav)) + len([]rune(gap)) + len([]rune(wild))
	pad := (width - coreW) / 2
	if pad < 0 {
		pad = 0
	}
	tw, ww := len([]rune(tav)), len([]rune(wild))
	absX := m.leftMargin + pad
	m.modeSpans = []modeSpan{
		{x0: absX, x1: absX + tw, wilds: false},
		{x0: absX + tw + len([]rune(gap)), x1: absX + tw + len([]rune(gap)) + ww, wilds: true},
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(styledWord(tav, litTav))
	b.WriteString(gap)
	b.WriteString(styledWord(wild, litWild))
	if !m.hideHoverTips {
		b.WriteString(ui.StyleMuted.Render("  ⌃G"))
	}
	// Right-aligned "F1 help" (clickable) on the header row, for consistency with
	// the detail views — the copy toast borrows the same slot while it's active.
	help := ui.StyleMuted.Render("F1 help")
	if m.toastActive() {
		help = m.renderToast()
	}
	slack := width - lipgloss.Width(b.String()) - lipgloss.Width(help)
	if slack < 1 {
		slack = 1
	}
	m.tavernHelpX = m.leftMargin + lipgloss.Width(b.String()) + slack
	m.tavernHelpWidth = lipgloss.Width(ui.StyleMuted.Render("F1 help"))
	if m.toastActive() {
		m.tavernHelpWidth = 0 // the toast isn't a button
	}
	b.WriteString(strings.Repeat(" ", slack))
	b.WriteString(help)
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
// end jump. The filter line (Wilds chips / open search bar) stays put.
func (m *Model) renderTransitionView() string {
	// Camp⇄Tavern: the Tavern rests at a fixed, top-anchored height (footer at the
	// bottom), not the centered content-sized block the reveal/dissolve otherwise
	// draws. So render the Tavern-side phase in that resting shape, opened by how
	// far the phase has run — the reveal lands exactly on the resting Tavern (and
	// the dissolve starts from it), with no end-of-transition height jump.
	if m.transAbsolute && m.transKind == kindMode {
		if m.transPhase == transReveal && m.inTavern() {
			return m.renderTavernOpenFrame(m.revealProgress())
		}
		if m.transPhase == transDissolve && !m.inTavern() {
			return m.renderTavernOpenFrame(1 - m.listFraction())
		}
	}
	width := m.contentWidth()
	m.leftMargin = (m.width - width) / 2
	if m.leftMargin < 0 {
		m.leftMargin = 0
	}
	margin := strings.Repeat(" ", m.leftMargin)

	litTav, litAfi := m.animatedModeLetters()
	headLine := m.renderModeLine(width, litTav, litAfi)
	if m.venturing() {
		headLine = m.ventureHeaderLine(width) // Camp→Wilds: swap straight to the WILDS header
	}
	header := []string{
		headLine,
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

	// Camp⇄Tavern body lines already carry their margin; everything else is
	// centered in the single column and gets the margin prepended here.
	rowMargin := margin
	if m.transAbsolute {
		rowMargin = ""
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
		b.WriteString(clip.Render(rowMargin+line) + "\n")
	}
	if overflow {
		b.WriteString(foldHint(margin, width) + "\n")
	}
	return b.String()
}

// renderTavernOpenFrame draws one Camp⇄Tavern frame with the Tavern in its resting
// fixed-height shape, opened by `openness`: 0 is the collapsed, centered header the
// pause leaves behind; 1 is the full resting Tavern — header pinned near the top,
// body filling colBodyH, footer at the bottom (identical to renderTavernView). The
// reveal runs it 0→1 and the dissolve 1→0, so the block irises open into / shut out
// of the resting layout instead of snapping to the taller height at the end.
func (m *Model) renderTavernOpenFrame(openness float64) string {
	if openness < 0 {
		openness = 0
	}
	if openness > 1 {
		openness = 1
	}

	contentW := clampInt(m.width-4, 40, 130)
	outer := max0((m.width - contentW) / 2)
	m.leftMargin = outer
	margin := strings.Repeat(" ", outer)

	litTav, litWild := m.animatedModeLetters()
	header := []string{
		m.renderModeLine(contentW, litTav, litWild),
		ui.CenterText(ui.StyleMuted.Render(m.transitionSubtitle()), contentW),
	}
	footer := indentLines(m.statusBar(contentW), margin)

	const bodyGap, footerGap = 3, 2
	comfy := clampInt(m.height/8, 2, 4)
	chrome := len(header) + bodyGap + footerGap + lipgloss.Height(footer)
	colBodyH := m.height - chrome - 2*comfy
	if colBodyH < 1 {
		colBodyH = 1
	}

	// The collapsed header sits where the centered pause leaves it; the open header
	// pins at comfy (the resting Tavern's top pad). Glide between the two so the
	// header rises/falls smoothly rather than snapping.
	availableHeight := m.height - 1
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
	collapsedTop := vpad + (innerHeight-(len(header)+3))/2
	if collapsedTop < 0 {
		collapsedTop = 0
	}
	topPad := lerpInt(collapsedTop, comfy, openness)

	// The body region grows from nothing to the full colBodyH, carrying the footer
	// down with it — but never shorter than the rows already on screen.
	rowLines := m.transitionRows()
	if len(rowLines) > colBodyH {
		rowLines = rowLines[:colBodyH]
	}
	regionH := lerpInt(0, colBodyH, openness)
	if regionH < len(rowLines) {
		regionH = len(rowLines)
	}
	if regionH > colBodyH {
		regionH = colBodyH
	}

	clip := lipgloss.NewStyle().MaxWidth(m.width)
	var b strings.Builder
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}
	for _, line := range header {
		b.WriteString(clip.Render(margin+line) + "\n")
	}
	b.WriteString(strings.Repeat("\n", bodyGap))
	m.modeToggleRow = topPad
	for i := 0; i < regionH; i++ {
		line := ""
		if i < len(rowLines) {
			line = rowLines[i] // Camp⇄Tavern rows carry their own margin (transAbsolute)
		}
		b.WriteString(clip.Render(line) + "\n")
	}
	b.WriteString(strings.Repeat("\n", footerGap))
	b.WriteString(footer)
	return b.String()
}
