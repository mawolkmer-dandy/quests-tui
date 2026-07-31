package lab

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/charmbracelet/harmonica"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The preview stage every effect paints onto. Fixed so the params panel below
// it never jumps between effects.
const (
	stageW = 52
	stageH = 7
	fps    = 60
	dt     = 1.0 / fps
)

// Param is one live-tunable knob shown in the params panel; the lab adjusts
// Val with ←/→ and the winning numbers get baked into the real app.
type Param struct {
	Name     string
	Val      float64
	Min, Max float64
	Step     float64
	Fmt      string // printf verb, default "%.2f"
}

func (p *Param) adjust(dir float64) {
	p.Val += dir * p.Step
	p.Val = math.Round(p.Val/p.Step) * p.Step
	if p.Val < p.Min {
		p.Val = p.Min
	}
	if p.Val > p.Max {
		p.Val = p.Max
	}
}

func (p *Param) str() string {
	f := p.Fmt
	if f == "" {
		f = "%.2f"
	}
	return fmt.Sprintf(f, p.Val)
}

// Effect is one animation in the gallery.
type Effect interface {
	Name() string
	Params() []*Param
	Trigger()
	Tick()
	Playing() bool
	Render() []string
}

// --- shared particle system --------------------------------------------------

type particle struct {
	x, y, vx, vy, life float64
	g                  rune
}

var sparkleGlyphs = []rune{'✦', '✧', '˖', '·'}

func spawnBurst(cx, cy float64, n int, speed float64) []particle {
	ps := make([]particle, 0, n)
	for i := 0; i < n; i++ {
		ang := rand.Float64() * 2 * math.Pi
		sp := speed * (0.5 + rand.Float64())
		ps = append(ps, particle{
			x: cx, y: cy,
			vx:   math.Cos(ang) * sp,
			vy:   math.Sin(ang) * sp * 0.5, // halve vertical — cells are ~2:1
			life: 1,
			g:    sparkleGlyphs[rand.Intn(2)],
		})
	}
	return ps
}

func spawnRing(cx, cy float64, n int, speed float64) []particle {
	ps := make([]particle, 0, n)
	for i := 0; i < n; i++ {
		ang := float64(i) / float64(n) * 2 * math.Pi
		ps = append(ps, particle{
			x: cx, y: cy,
			vx:   math.Cos(ang) * speed,
			vy:   math.Sin(ang) * speed * 0.5,
			life: 1,
			g:    '✦',
		})
	}
	return ps
}

func advance(ps []particle, decay float64) []particle {
	out := ps[:0]
	for _, p := range ps {
		p.x += p.vx * dt
		p.y += p.vy * dt
		p.life -= decay * dt
		if p.life > 0 {
			out = append(out, p)
		}
	}
	return out
}

func drawParticles(g *grid, ps []particle) {
	for _, p := range ps {
		glyph := p.g
		col := cGold
		switch {
		case p.life < 0.33:
			glyph, col = '·', cMuted
		case p.life < 0.66:
			glyph = '˖'
		}
		g.set(int(p.x+0.5), int(p.y+0.5), glyph, col)
	}
}

func spring(freq, damping float64) harmonica.Spring {
	return harmonica.NewSpring(harmonica.FPS(fps), freq, damping)
}

func settled(pos, vel, target float64) bool {
	return math.Abs(pos-target) < 0.005 && math.Abs(vel) < 0.005
}

// --- 1. Objective: check pop -------------------------------------------------

type checkPop struct {
	freq, damping, sparkles, decay, flash *Param
	pos, vel, t                           float64
	ps                                    []particle
	playing                               bool
}

func newCheckPop() *checkPop {
	return &checkPop{
		freq:     &Param{Name: "spring freq", Val: 8, Min: 2, Max: 20, Step: 0.5},
		damping:  &Param{Name: "damping", Val: 0.32, Min: 0.1, Max: 1.2, Step: 0.02},
		sparkles: &Param{Name: "sparkles", Val: 9, Min: 0, Max: 20, Step: 1, Fmt: "%.0f"},
		decay:    &Param{Name: "spark fade (1/s)", Val: 3.2, Min: 0.4, Max: 8, Step: 0.2},
		flash:    &Param{Name: "green flash (s)", Val: 0.45, Min: 0, Max: 2, Step: 0.05},
	}
}

func (e *checkPop) Name() string { return "Check pop" }
func (e *checkPop) Params() []*Param {
	return []*Param{e.freq, e.damping, e.sparkles, e.decay, e.flash}
}
func (e *checkPop) Playing() bool { return e.playing }
func (e *checkPop) Trigger() {
	e.pos, e.vel, e.t, e.playing = 0, 0, 0, true
	e.ps = spawnBurst(9, 3, int(e.sparkles.Val), 22)
}
func (e *checkPop) Tick() {
	if !e.playing {
		return
	}
	sp := spring(e.freq.Val, e.damping.Val)
	e.pos, e.vel = sp.Update(e.pos, e.vel, 1)
	e.ps = advance(e.ps, e.decay.Val)
	e.t += dt
	if settled(e.pos, e.vel, 1) && len(e.ps) == 0 && e.t > e.flash.Val {
		e.playing = false
	}
}
func (e *checkPop) Render() []string {
	g := newGrid(stageW, stageH)
	y := 3
	drawParticles(g, e.ps) // sparkles behind, so the text stays legible
	box, boxColor := boxGlyph(e.pos)
	g.set(6, y, box, boxColor)
	// overshoot flourish: tiny brackets hugging the box at the peak
	if e.pos > 1.04 {
		g.set(4, y, '❲', cGold)
		g.set(8, y, '❳', cGold)
	}
	textColor := cMuted
	if e.playing && e.t < e.flash.Val {
		textColor = cGreen
	} else if e.pos > 0.85 {
		textColor = cGreen
	}
	g.text(10, y, "Gather shards", textColor)
	return g.render()
}

// boxGlyph maps a 0→1(→overshoot) spring value onto the objective checkbox
// growing from hollow to filled.
func boxGlyph(pos float64) (rune, int8) {
	switch {
	case pos < 0.3:
		return []rune(ui.GlyphQuestOpen)[0], cMuted
	case pos < 0.85:
		return []rune(ui.GlyphQuestActive)[0], cGreen
	default:
		return []rune(ui.GlyphQuestDone)[0], cGreen
	}
}

// --- 2. Objective: strike & collapse ----------------------------------------

type strikeCollapse struct {
	freq, damping, hold *Param
	pos, vel, t         float64
	playing             bool
}

func newStrikeCollapse() *strikeCollapse {
	return &strikeCollapse{
		freq:    &Param{Name: "collapse freq", Val: 4.5, Min: 2, Max: 20, Step: 0.5},
		damping: &Param{Name: "damping", Val: 0.9, Min: 0.1, Max: 1.2, Step: 0.05},
		hold:    &Param{Name: "flash hold (s)", Val: 0.35, Min: 0, Max: 1.5, Step: 0.05},
	}
}

func (e *strikeCollapse) Name() string     { return "Strike & collapse" }
func (e *strikeCollapse) Params() []*Param { return []*Param{e.freq, e.damping, e.hold} }
func (e *strikeCollapse) Playing() bool    { return e.playing }
func (e *strikeCollapse) Trigger()         { e.pos, e.vel, e.t, e.playing = 0, 0, 0, true }
func (e *strikeCollapse) Tick() {
	if !e.playing {
		return
	}
	e.t += dt
	if e.t >= e.hold.Val { // collapse only after the flash hold
		sp := spring(e.freq.Val, e.damping.Val)
		e.pos, e.vel = sp.Update(e.pos, e.vel, 1)
		if settled(e.pos, e.vel, 1) {
			e.playing = false
		}
	}
}
func (e *strikeCollapse) Render() []string {
	g := newGrid(stageW, stageH)
	base := 1
	const target = "Gather shards"
	g.text(6, base, ui.GlyphQuestOpen+" Melt the glass", cMuted)

	// The target holds (green flash), then its text wipes away right→left as the
	// spring runs; once wiped, the rows below close up into its slot.
	collapsed := e.pos > 0.92
	if !collapsed {
		tr := []rune(target)
		reveal := len(tr)
		if e.t >= e.hold.Val {
			p := e.pos
			if p < 0 {
				p = 0
			}
			if p > 1 {
				p = 1
			}
			reveal = int(float64(len(tr))*(1-p) + 0.5)
		}
		g.set(6, base+1, []rune(ui.GlyphQuestDone)[0], cGreen)
		g.text(8, base+1, string(tr[:reveal]), cGreen)
	}
	shift := 0
	if collapsed {
		shift = 1
	}
	g.text(6, base+2-shift, ui.GlyphQuestOpen+" Reforge the seal", cMuted)
	g.text(6, base+3-shift, ui.GlyphQuestOpen+" Polish the rim", cMuted)
	return g.render()
}

// --- 3. Quest: seal stamp ----------------------------------------------------

type sealStamp struct {
	freq, damping, ring, decay *Param
	pos, vel, t                float64
	ps                         []particle
	rung                       bool
	playing                    bool
}

func newSealStamp() *sealStamp {
	return &sealStamp{
		freq:    &Param{Name: "stamp freq", Val: 6, Min: 2, Max: 20, Step: 0.5},
		damping: &Param{Name: "damping", Val: 0.4, Min: 0.1, Max: 1.2, Step: 0.02},
		ring:    &Param{Name: "ring sparks", Val: 10, Min: 0, Max: 20, Step: 1, Fmt: "%.0f"},
		decay:   &Param{Name: "spark fade (1/s)", Val: 3.2, Min: 0.4, Max: 8, Step: 0.2},
	}
}

func (e *sealStamp) Name() string     { return "Seal stamp" }
func (e *sealStamp) Params() []*Param { return []*Param{e.freq, e.damping, e.ring, e.decay} }
func (e *sealStamp) Playing() bool    { return e.playing }
func (e *sealStamp) Trigger()         { e.pos, e.vel, e.t, e.playing, e.rung, e.ps = 0, 0, 0, true, false, nil }
func (e *sealStamp) Tick() {
	if !e.playing {
		return
	}
	sp := spring(e.freq.Val, e.damping.Val)
	e.pos, e.vel = sp.Update(e.pos, e.vel, 1)
	if !e.rung && e.pos > 0.8 { // fire the flourish ring once, at the stamp's peak
		e.ps = spawnRing(6, 3, int(e.ring.Val), 26)
		e.rung = true
	}
	e.ps = advance(e.ps, e.decay.Val)
	e.t += dt
	if settled(e.pos, e.vel, 1) && len(e.ps) == 0 {
		e.playing = false
	}
}
func (e *sealStamp) Render() []string {
	g := newGrid(stageW, stageH)
	y := 3
	var glyph rune
	switch {
	case e.pos < 0.3:
		glyph = []rune(ui.GlyphQuestActive)[0]
	case e.pos > 1.04:
		glyph = []rune(ui.GlyphOrnament)[0] // ❖ overshoot peak reads "bigger"
	default:
		glyph = []rune(ui.GlyphQuestDone)[0]
	}
	drawParticles(g, e.ps) // flourish ring behind the seal + title
	g.set(6, y, glyph, cGold)
	// title crossfades name → gold flash → muted "done"
	titleColor := cName
	if e.pos > 0.85 {
		titleColor = cGold
	}
	if !e.playing {
		titleColor = cMuted
	}
	g.text(8, y, "Repair the phial", titleColor)
	return g.render()
}

// --- 4. Quest: shimmer sweep -------------------------------------------------

type shimmerSweep struct {
	speed, band *Param
	pos         float64
	playing     bool
}

func newShimmerSweep() *shimmerSweep {
	return &shimmerSweep{
		speed: &Param{Name: "speed (cols/s)", Val: 34, Min: 6, Max: 90, Step: 2, Fmt: "%.0f"},
		band:  &Param{Name: "band width", Val: 5, Min: 1, Max: 16, Step: 1, Fmt: "%.0f"},
	}
}

func (e *shimmerSweep) Name() string     { return "Shimmer sweep" }
func (e *shimmerSweep) Params() []*Param { return []*Param{e.speed, e.band} }
func (e *shimmerSweep) Playing() bool    { return e.playing }
func (e *shimmerSweep) Trigger()         { e.pos, e.playing = -e.band.Val, true }
func (e *shimmerSweep) Tick() {
	if !e.playing {
		return
	}
	e.pos += e.speed.Val * dt
}
func (e *shimmerSweep) Render() []string {
	g := newGrid(stageW, stageH)
	const title = "◆ Repair the phial"
	x0, y := 6, 3
	half := e.band.Val / 2
	end := true
	for i, r := range []rune(title) {
		col := cName
		d := math.Abs(float64(i) - e.pos)
		if d <= half {
			col = cGold // inside the sweep band
			end = false
		} else if float64(i) > e.pos {
			end = false
		}
		g.set(x0+i, y, r, col)
	}
	if end && e.playing {
		e.playing = false
	}
	return g.render()
}

// --- 5. New quest: grow-in ---------------------------------------------------

type growIn struct {
	freq, damping, typeCPS *Param
	pos, vel, t            float64
	playing                bool
}

func newGrowIn() *growIn {
	return &growIn{
		freq:    &Param{Name: "slide freq", Val: 7, Min: 2, Max: 20, Step: 0.5},
		damping: &Param{Name: "damping", Val: 0.6, Min: 0.1, Max: 1.2, Step: 0.05},
		typeCPS: &Param{Name: "type (chars/s)", Val: 42, Min: 6, Max: 120, Step: 3, Fmt: "%.0f"},
	}
}

func (e *growIn) Name() string     { return "Grow-in" }
func (e *growIn) Params() []*Param { return []*Param{e.freq, e.damping, e.typeCPS} }
func (e *growIn) Playing() bool    { return e.playing }
func (e *growIn) Trigger()         { e.pos, e.vel, e.t, e.playing = 0, 0, 0, true }
func (e *growIn) Tick() {
	if !e.playing {
		return
	}
	sp := spring(e.freq.Val, e.damping.Val)
	e.pos, e.vel = sp.Update(e.pos, e.vel, 1)
	e.t += dt
	const title = "Scout the ridge"
	typed := int(e.t * e.typeCPS.Val)
	if settled(e.pos, e.vel, 1) && typed >= len(title) {
		e.playing = false
	}
}
func (e *growIn) Render() []string {
	g := newGrid(stageW, stageH)
	const title = "Scout the ridge"
	y := 3
	// slides in from the left as the spring settles; fades muted → name
	xOff := int((1 - e.pos) * 6)
	if xOff < 0 {
		xOff = 0
	}
	col := cMuted
	if e.pos > 0.75 {
		col = cName
	}
	box := []rune(ui.GlyphQuestActive)[0]
	g.set(6+xOff, y, box, cGreen)
	typed := int(e.t * e.typeCPS.Val)
	if typed > len(title) {
		typed = len(title)
	}
	g.text(8+xOff, y, title[:typed], col)
	if typed < len(title) {
		g.set(8+xOff+typed, y, '▏', cMuted) // typing caret
	}
	return g.render()
}

// --- 6. Cursor: springy cursor ----------------------------------------------

type springCursor struct {
	freq, damping *Param
	posY, vel     float64
	target        int
	rows          []string
	playing       bool
}

func newSpringCursor() *springCursor {
	return &springCursor{
		freq:    &Param{Name: "cursor freq", Val: 12, Min: 2, Max: 30, Step: 0.5},
		damping: &Param{Name: "damping", Val: 0.75, Min: 0.1, Max: 1.2, Step: 0.05},
		rows:    []string{"Repair the phial", "Scout the ridge", "Gather shards", "Melt the glass", "Reforge the seal"},
	}
}

func (e *springCursor) Name() string     { return "Springy cursor" }
func (e *springCursor) Params() []*Param { return []*Param{e.freq, e.damping} }
func (e *springCursor) Playing() bool    { return e.playing }

// Trigger advances the cursor to the next row (wrapping) so each replay press
// shows the ease between positions.
func (e *springCursor) Trigger() {
	e.target = (e.target + 1) % len(e.rows)
	e.playing = true
}
func (e *springCursor) Tick() {
	if !e.playing {
		return
	}
	sp := spring(e.freq.Val, e.damping.Val)
	e.posY, e.vel = sp.Update(e.posY, e.vel, float64(e.target))
	if settled(e.posY, e.vel, float64(e.target)) {
		e.playing = false
	}
}
func (e *springCursor) Render() []string {
	g := newGrid(stageW, stageH)
	base := 1
	for i, r := range e.rows {
		col := cMuted
		if i == e.target {
			col = cName
		}
		g.text(4, base+i, r, col)
	}
	// cursor glyph at the interpolated row, plus a faint ghost on the row it's
	// crossing so the motion reads between cells.
	cy := e.posY
	yi := int(cy + 0.5)
	g.text(2, base+yi, ui.GlyphCursor, cGold)
	frac := cy - math.Floor(cy)
	if frac > 0.15 && frac < 0.85 {
		g.set(2, base+int(math.Floor(cy)), '·', cMuted)
		g.set(2, base+int(math.Ceil(cy)), '·', cMuted)
	}
	return g.render()
}

// centeredStage vertically centers already-styled lines in the fixed stage.
func centeredStage(content []string) []string {
	out := make([]string, stageH)
	top := (stageH - len(content)) / 2
	if top < 0 {
		top = 0
	}
	for i, l := range content {
		if top+i < stageH {
			out[top+i] = l
		}
	}
	return out
}

var sampleRows = []string{"Repair the phial", "Scout the ridge", "Gather shards", "Melt the glass", "Reforge the seal"}

// --- current (in-app): intro banner — shine sweep + subtitle typewriter ------

type introBanner struct {
	speed   *Param
	f       float64
	sub     string
	playing bool
}

func newIntroBanner() *introBanner {
	return &introBanner{
		speed: &Param{Name: "frames/s", Val: 20, Min: 4, Max: 60, Step: 2, Fmt: "%.0f"},
		sub:   ui.TavernGreetings[0],
	}
}

func (e *introBanner) Name() string     { return "Intro banner (shine+type)" }
func (e *introBanner) Params() []*Param { return []*Param{e.speed} }
func (e *introBanner) Playing() bool    { return e.playing }
func (e *introBanner) Trigger()         { e.f, e.playing = 0, true }
func (e *introBanner) Tick() {
	if !e.playing {
		return
	}
	e.f += e.speed.Val * dt
	if int(e.f) >= ui.IntroTotalFrames(e.sub) {
		e.playing = false
	}
}
func (e *introBanner) Render() []string {
	f := int(e.f)
	if total := ui.IntroTotalFrames(e.sub); f > total {
		f = total
	}
	return centeredStage(ui.RenderLogoIntro(stageW, e.sub, f))
}

// --- current (in-app): list reveal — staggered slide-in ----------------------

type listReveal struct {
	stagger, slide *Param
	t              float64
	playing        bool
}

func newListReveal() *listReveal {
	return &listReveal{
		stagger: &Param{Name: "stagger (s)", Val: 0.07, Min: 0, Max: 0.4, Step: 0.01},
		slide:   &Param{Name: "slide-in (s)", Val: 0.14, Min: 0.02, Max: 0.6, Step: 0.02},
	}
}

func (e *listReveal) Name() string     { return "List reveal" }
func (e *listReveal) Params() []*Param { return []*Param{e.stagger, e.slide} }
func (e *listReveal) Playing() bool    { return e.playing }
func (e *listReveal) Trigger()         { e.t, e.playing = 0, true }
func (e *listReveal) Tick() {
	if !e.playing {
		return
	}
	e.t += dt
	last := float64(len(sampleRows)-1)*e.stagger.Val + e.slide.Val
	if e.t >= last {
		e.playing = false
	}
}
func (e *listReveal) Render() []string {
	g := newGrid(stageW, stageH)
	for i, r := range sampleRows {
		age := e.t - float64(i)*e.stagger.Val
		if age < 0 {
			continue // not revealed yet
		}
		p := age / e.slide.Val
		if p > 1 {
			p = 1
		}
		xOff := int((1 - p) * 5)
		col := cMuted
		if p > 0.7 {
			col = cName
		}
		g.set(4+xOff, 1+i, []rune(ui.GlyphQuestActive)[0], cGreen)
		g.text(6+xOff, 1+i, r, col)
	}
	return g.render()
}

// --- current (in-app): list dissolve — right→left burn -----------------------

type listDissolve struct {
	speed   *Param
	t       float64
	playing bool
}

func newListDissolve() *listDissolve {
	return &listDissolve{
		speed: &Param{Name: "burn (cols/s)", Val: 40, Min: 8, Max: 120, Step: 4, Fmt: "%.0f"},
	}
}

func (e *listDissolve) Name() string     { return "List dissolve" }
func (e *listDissolve) Params() []*Param { return []*Param{e.speed} }
func (e *listDissolve) Playing() bool    { return e.playing }
func (e *listDissolve) Trigger()         { e.t, e.playing = 0, true }
func (e *listDissolve) Tick() {
	if !e.playing {
		return
	}
	e.t += dt
	maxLen := 0
	for _, r := range sampleRows {
		if n := len([]rune(r)) + 2; n > maxLen {
			maxLen = n
		}
	}
	if int(e.t*e.speed.Val) >= maxLen {
		e.playing = false
	}
}
func (e *listDissolve) Render() []string {
	g := newGrid(stageW, stageH)
	cut := int(e.t * e.speed.Val) // columns burned away from the right
	for i, r := range sampleRows {
		full := []rune(ui.GlyphQuestActive + " " + r)
		keep := len(full) - cut
		if keep < 0 {
			keep = 0
		}
		col := cMuted
		g.set(4, 1+i, full[0], cGreen)
		if keep > 2 {
			g.text(6, 1+i, string(full[2:keep]), col)
		} else if keep <= 0 {
			g.set(4, 1+i, ' ', cDefault) // fully burned: even the glyph is gone
		}
	}
	return g.render()
}
