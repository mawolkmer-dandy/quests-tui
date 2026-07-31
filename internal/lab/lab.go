// Package lab is a flag-gated animation gallery (`quests --lab`) for designing
// and tuning the app's interactions — completing objectives/quests, adding
// quests, cursor motion — against the real internal/ui styles and glyphs, so a
// dialed-in effect ports straight into the app. Not shipped to normal use.
package lab

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// entry is one effect plus whether it's already wired into the real app
// ("live") versus a proposal still being designed ("new").
type entry struct {
	e    Effect
	live bool
}

type group struct {
	name    string
	entries []entry
}

type frameMsg struct{}

func frame() tea.Cmd {
	return tea.Tick(time.Second/fps, func(time.Time) tea.Msg { return frameMsg{} })
}

const (
	focusList = iota
	focusParams
)

// Model is the lab gallery.
type Model struct {
	groups []group
	flat   []entry
	sel    int // index into flat
	focus  int
	selP   int // selected param within the current effect
	w, h   int
}

// New builds the gallery, grouped by the action each effect animates. The last
// group catalogs the animations already live in the app; everything above it
// is a proposal being designed.
func New() *Model {
	nu := func(e Effect) entry { return entry{e: e} }
	live := func(e Effect) entry { return entry{e: e, live: true} }
	groups := []group{
		{"Objective complete", []entry{nu(newCheckPop()), nu(newStrikeCollapse())}},
		{"Quest complete", []entry{nu(newSealStamp()), nu(newShimmerSweep())}},
		{"New quest", []entry{nu(newGrowIn())}},
		{"Cursor", []entry{nu(newSpringCursor())}},
		{"Current — in the app", []entry{live(newIntroBanner()), live(newListReveal()), live(newListDissolve())}},
	}
	var flat []entry
	for _, g := range groups {
		flat = append(flat, g.entries...)
	}
	return &Model{groups: groups, flat: flat}
}

func (m *Model) cur() Effect { return m.flat[m.sel].e }

func (m *Model) Init() tea.Cmd {
	m.cur().Trigger()
	return frame()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case frameMsg:
		for _, en := range m.flat {
			en.e.Tick()
		}
		return m, frame()
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, nil
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return tea.Quit
	case "tab":
		if m.focus == focusList {
			m.focus = focusParams
		} else {
			m.focus = focusList
		}
	case " ", "enter", "r":
		m.cur().Trigger()
	case "a":
		for _, en := range m.flat {
			en.e.Trigger()
		}
	case "up":
		if m.focus == focusParams {
			if m.selP > 0 {
				m.selP--
			}
		} else if m.sel > 0 {
			m.sel--
			m.selP = 0
			m.cur().Trigger()
		}
	case "down":
		if m.focus == focusParams {
			if m.selP < len(m.cur().Params())-1 {
				m.selP++
			}
		} else if m.sel < len(m.flat)-1 {
			m.sel++
			m.selP = 0
			m.cur().Trigger()
		}
	case "left", "[":
		m.adjust(-1)
	case "right", "]":
		m.adjust(1)
	}
	return nil
}

func (m *Model) adjust(dir float64) {
	if m.focus != focusParams {
		m.focus = focusParams
	}
	ps := m.cur().Params()
	if m.selP < len(ps) {
		ps[m.selP].adjust(dir)
		m.cur().Trigger() // replay so the change is felt immediately
	}
}

func (m *Model) View() tea.View {
	if m.w == 0 {
		return tea.NewView("")
	}
	var b strings.Builder
	b.WriteString(ui.StyleTitle.Render("QUESTS · LAB") + "  " + ui.StyleMuted.Render("animation gallery"))
	b.WriteString("\n")
	b.WriteString(ui.StyleMuted.Render("↑↓ pick · space replay · tab focus params · ←→ adjust · a play all · q quit"))
	b.WriteString("\n\n")

	left := m.leftColumn()
	right := m.rightColumn()
	b.WriteString(zip(left, right, 32))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m *Model) leftColumn() []string {
	var lines []string
	liveTag := lipgloss.NewStyle().Foreground(ui.ColorHeading).Render(" live")
	newTag := ui.StyleMuted.Render(" new")
	fi := 0
	for _, g := range m.groups {
		lines = append(lines, ui.StyleSectionHeader.Render(g.name))
		for _, en := range g.entries {
			mark := "  "
			name := en.e.Name()
			if fi == m.sel {
				mark = ui.StyleCursor.Render(ui.GlyphCursor)
				if m.focus == focusList {
					name = ui.StyleSelectedRow.Render(" " + name + " ")
				} else {
					name = ui.StyleName.Render(name)
				}
			} else {
				name = ui.StyleMuted.Render(name)
			}
			dot := " "
			if en.e.Playing() {
				dot = lipgloss.NewStyle().Foreground(ui.ColorAccent).Render("●")
			}
			tag := newTag
			if en.live {
				tag = liveTag
			}
			lines = append(lines, fmt.Sprintf("%s%s %s%s", mark, dot, name, tag))
			fi++
		}
		lines = append(lines, "")
	}
	return lines
}

func (m *Model) rightColumn() []string {
	var lines []string
	lines = append(lines, ui.StyleMuted.Render("preview"))
	// preview stage, boxed for a stable frame
	stage := m.cur().Render()
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorSide).
		Padding(0, 1).
		Render(strings.Join(stage, "\n"))
	lines = append(lines, strings.Split(box, "\n")...)
	lines = append(lines, "")

	label := "params"
	if m.focus == focusParams {
		label = ui.StyleName.Render("params") + ui.StyleMuted.Render("  (←→ adjust)")
	} else {
		label = ui.StyleMuted.Render("params  (tab to adjust)")
	}
	lines = append(lines, label)
	for i, p := range m.cur().Params() {
		mark := "  "
		nameCol := ui.StyleMuted
		if m.focus == focusParams && i == m.selP {
			mark = ui.StyleCursor.Render(ui.GlyphCursor)
			nameCol = ui.StyleName
		}
		valStyle := lipgloss.NewStyle().Foreground(ui.ColorAccent)
		lines = append(lines, fmt.Sprintf("%s%s  %s", mark, nameCol.Render(pad(p.Name, 16)), valStyle.Render(p.str())))
	}
	return lines
}

func pad(s string, w int) string {
	if lipgloss.Width(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

// zip lays two columns side by side, padding the left one to leftW and the
// shorter to match the taller.
func zip(left, right []string, leftW int) string {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lw := lipgloss.Width(l)
		if lw < leftW {
			l += strings.Repeat(" ", leftW-lw)
		}
		b.WriteString("  " + l + "  " + r + "\n")
	}
	return b.String()
}

// Run launches the lab program. darkBg must already have been resolved and
// ui.Init called by the caller.
func Run() error {
	_, err := tea.NewProgram(New()).Run()
	return err
}
