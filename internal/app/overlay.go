package app

import (
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Screen-space composition layer. Transient effects (sparkle bursts today,
// cursor trails / stamps later) are positioned by ABSOLUTE screen (x,y) and
// painted on top of the final rendered frame in View() — so they appear over
// any surface (Tavern, Wilds, detail page) regardless of which renderer drew
// the row underneath. This decouples animation from the per-surface row
// renderers: a caller just spawns an effect at a screen cell.

const overlayFPS = 60

// Columns of a row's icon (status glyph / checkbox) relative to its cursor
// cell (cursorScreenX = the "›" marker column), so a completion burst emanates
// from the exact icon. They differ because each row kind lays its icon out
// differently (see RenderRow / renderBodyLineWrapped).
const (
	questGlyphCol = 4 // RowQuest: cursorMark(2) + priorityIndicator(2)
	wildsObjCol   = 8 // RowWildsObjective: cursorMark(2) + indent(6); +2 per nest
	bodyObjCol    = 2 // detail body objective: cursorMark(2); +2 per nest level
)

type overlayParticle struct {
	x, y, vx, vy float64
	life         float64 // 1 → 0
	decay        float64 // life units consumed per second
	glyph        rune
	trail        bool // a cursor-trail ghost (renders its own glyph, not a sparkle)
}

type overlayTickMsg struct{ gen int }

func overlayTick(gen int) tea.Cmd {
	return tea.Tick(time.Second/overlayFPS, func(time.Time) tea.Msg { return overlayTickMsg{gen: gen} })
}

// spawnSparkleBurst scatters n sparkles out of a screen cell — the completion
// "pop". Vertical speed is halved because terminal cells are ~2:1 tall.
func (m *Model) spawnSparkleBurst(x, y, n int) {
	if x < 0 || y < 0 || n <= 0 {
		return
	}
	for i := 0; i < n; i++ {
		ang := rand.Float64() * 2 * math.Pi
		sp := 16.0 * (0.5 + rand.Float64())
		m.overlayParticles = append(m.overlayParticles, overlayParticle{
			x: float64(x), y: float64(y),
			vx:    math.Cos(ang) * sp,
			vy:    math.Sin(ang) * sp * 0.5,
			life:  1,
			decay: 3.4,
			glyph: '✦',
		})
	}
}

// spawnSparkleRing emits an evenly-spaced radial ring of sparkles — the quest
// "seal stamp" flourish (vs. the scattered burst for objectives).
func (m *Model) spawnSparkleRing(x, y, n int) {
	if x < 0 || y < 0 || n <= 0 {
		return
	}
	for i := 0; i < n; i++ {
		ang := float64(i) / float64(n) * 2 * math.Pi
		const sp = 20.0
		m.overlayParticles = append(m.overlayParticles, overlayParticle{
			x: float64(x), y: float64(y),
			vx:    math.Cos(ang) * sp,
			vy:    math.Sin(ang) * sp * 0.5,
			life:  1,
			decay: 3.0,
			glyph: '✦',
		})
	}
}

// spawnCursorTrail drops a fading "›" ghost where the cursor just was — the
// cursor itself moves instantly (no easing), leaving a short trail behind.
func (m *Model) spawnCursorTrail(x, y int) {
	if x < 0 || y < 0 {
		return
	}
	m.overlayParticles = append(m.overlayParticles, overlayParticle{
		x: float64(x), y: float64(y),
		life:  1,
		decay: 6.0, // ~0.16s — a brief wisp, not a lingering smear
		glyph: []rune(ui.GlyphCursor)[0],
		trail: true,
	})
}

func (m *Model) maybeStartOverlayTick() tea.Cmd {
	if len(m.overlayParticles) == 0 || m.overlayTickOn {
		return nil
	}
	m.overlayTickOn = true
	m.overlayGen++
	return overlayTick(m.overlayGen)
}

// pokeOverlayTick starts the ticker even with no particles yet — used when an
// effect will be spawned during the upcoming render (a deferred connection
// burst), so the ticker is live to animate it once it appears.
func (m *Model) pokeOverlayTick() tea.Cmd {
	if m.overlayTickOn {
		return nil
	}
	m.overlayTickOn = true
	m.overlayGen++
	return overlayTick(m.overlayGen)
}

func (m *Model) onOverlayTick(gen int) tea.Cmd {
	if gen != m.overlayGen {
		return nil
	}
	const dt = 1.0 / overlayFPS
	alive := m.overlayParticles[:0]
	for _, p := range m.overlayParticles {
		p.x += p.vx * dt
		p.y += p.vy * dt
		p.life -= p.decay * dt
		if p.life > 0 {
			alive = append(alive, p)
		}
	}
	m.overlayParticles = alive
	m.invalidateRender()
	if len(m.overlayParticles) == 0 {
		m.overlayTickOn = false
		return nil
	}
	return overlayTick(gen)
}

// overlayGlyph builds the styled sparkle for a particle's remaining life. The
// gold style is created HERE (not a package var) because ui.ColorAccent is only
// resolved at runtime by ui.Init — capturing it at package-init would give a
// nil color, i.e. plain white.
func overlayGlyph(p overlayParticle) string {
	gold := lipgloss.NewStyle().Foreground(ui.ColorAccent)
	if p.trail { // cursor ghost: its own glyph, gold fading to muted
		if p.life > 0.5 {
			return gold.Render(string(p.glyph))
		}
		return ui.StyleMuted.Render(string(p.glyph))
	}
	switch {
	case p.life > 0.66:
		return gold.Render(string(p.glyph))
	case p.life > 0.33:
		return gold.Render("˖")
	default:
		return ui.StyleMuted.Render("·")
	}
}

// compositeOverlay paints active particles onto the final frame at their screen
// cells, leaving the underlying styled text untouched everywhere else.
func (m *Model) compositeOverlay(frame string) string {
	if len(m.overlayParticles) == 0 {
		return frame
	}
	byRow := map[int]map[int]string{}
	for _, p := range m.overlayParticles {
		x, y := int(p.x+0.5), int(p.y+0.5)
		if x < 0 || y < 0 {
			continue
		}
		if byRow[y] == nil {
			byRow[y] = map[int]string{}
		}
		byRow[y][x] = overlayGlyph(p)
	}
	lines := strings.Split(frame, "\n")
	for y, cells := range byRow {
		for y >= len(lines) {
			lines = append(lines, "")
		}
		lines[y] = overlayOnLine(lines[y], cells)
	}
	return strings.Join(lines, "\n")
}

// overlayOnLine replaces the runes at the given display columns of an
// ANSI-styled line with self-contained styled glyphs, re-emitting the line's
// active SGR after each so the surrounding styling doesn't drop (lipgloss
// emits set→content→reset spans, so tracking the last non-reset SGR suffices).
// Columns past the line's end are reached by padding with spaces.
func overlayOnLine(line string, cells map[int]string) string {
	var b strings.Builder
	rs := []rune(line)
	col := 0
	active := ""
	i := 0
	for i < len(rs) {
		if rs[i] == '\x1b' { // pass an ANSI escape through, tracking the active SGR
			j := i
			for j < len(rs) && rs[j] != 'm' {
				j++
			}
			if j < len(rs) {
				j++
			}
			seq := string(rs[i:j])
			if seq == "\x1b[0m" || seq == "\x1b[m" {
				active = ""
			} else {
				active = seq
			}
			b.WriteString(seq)
			i = j
			continue
		}
		if ov, ok := cells[col]; ok {
			// Paint the sparkle on top of whatever's underneath (it's an
			// overlay); the effect is brief so it doesn't obscure text for long.
			b.WriteString(ov)     // self-contained styled glyph (its own reset)
			b.WriteString(active) // restore the line's active style for following runes
		} else {
			b.WriteRune(rs[i])
		}
		col++
		i++
	}
	// Cells beyond the line's content: pad spaces out to each and place them.
	var beyond []int
	for c := range cells {
		if c >= col {
			beyond = append(beyond, c)
		}
	}
	sort.Ints(beyond)
	for _, c := range beyond {
		for col < c {
			b.WriteByte(' ')
			col++
		}
		b.WriteString(cells[c])
		col++
	}
	return b.String()
}
