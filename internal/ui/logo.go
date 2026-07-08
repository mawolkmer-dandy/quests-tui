package ui

import (
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TavernGreetings are candidate subtitles for the banner — one is picked at
// random each time the app starts (see RandomGreeting), not re-rolled every
// frame.
var TavernGreetings = []string{
	"Greetings, traveler!",
	"Welcome to the inn.",
	"Pull up a chair by the fire.",
	"The road is long — rest a while.",
	"What'll it be, adventurer?",
	"Another mug for the weary?",
	"The bard's about to start a new tale.",
	"Word is there's coin in these quests.",
	"The innkeeper nods as you enter.",
	"Fresh notices pinned by the door.",
	"Fresh bread, fresh rumors.",
	"You've wandered far to find this place.",
	"Sit, drink, and hear what's afoot.",
	"The fire crackles; the tales begin.",
	"Every legend starts with a single step.",
	"The Questboard's got a new face on it.",
	"Not all who wander are lost — check the board anyway.",
	"Adventure awaits beyond that door.",
	"May your blade stay sharp and your coin purse full.",
	"Rumor has it there's work to be done.",
	"Stay a while and listen.",
	"The innkeeper's heard tales of you already.",
	"Safe travels, whatever road you take.",
	"There's always another quest.",
	"The tavern hums with talk of work undone.",
	"A fire crackles. Your ledger waits.",
	"Ale in hand, you survey the board.",
	"Boots off — for now. The board's still full.",
	"The barkeep nods. Plenty to be done.",
	"Rumors and requests crowd the board.",
}

// AfieldGreetings are the subtitles shown out on the road (the Afield view),
// picked fresh each time you set out.
var AfieldGreetings = []string{
	"The road unspools before you.",
	"Boots on the trail, objectives ahead.",
	"No walls here — only the task.",
	"The wilds are patient. Your quests are not.",
	"Head down, blade ready.",
	"The tavern's behind you. Onward.",
	"Wind at your back, work ahead.",
	"Mud, miles, and a list to clear.",
	"Daylight's burning — move.",
	"One foot, then the next.",
}

// RandomGreeting picks one tavern subtitle line.
func RandomGreeting() string {
	return TavernGreetings[rand.Intn(len(TavernGreetings))]
}

// RandomAfieldGreeting picks one adventure subtitle line.
func RandomAfieldGreeting() string {
	return AfieldGreetings[rand.Intn(len(AfieldGreetings))]
}

// RenderLogo returns the small banner shown above the outline, centered
// within width.
func RenderLogo(width int, subtitle string) []string {
	title := StyleTitle.Render("QUESTS")
	sub := StyleMuted.Render(subtitle)

	return []string{centerText(title, width), centerText(sub, width)}
}

// IntroShineFrames is how long the startup shine sweeps across the title;
// IntroCharsPerTick is how fast the subtitle types in underneath it — both
// run at once, in place, on the very first render (see RenderLogoIntro).
const (
	IntroShineFrames  = 10
	IntroCharsPerTick = 2
)

// introShineHighlight is the brief highlight color the sweep passes through
// — the title itself stays exactly StyleTitle's plain bold color throughout,
// so there's no jump when the animation ends and RenderLogo takes over.
var introShineHighlight = lipgloss.AdaptiveColor{Light: "#B8860B", Dark: "#FFD54F"}

// IntroTotalFrames is how many frames the intro needs — until both the
// shine sweep and the subtitle's typewriter reveal have finished, whichever
// takes longer.
func IntroTotalFrames(subtitle string) int {
	subtitleFrames := (len([]rune(subtitle)) + IntroCharsPerTick - 1) / IntroCharsPerTick
	total := IntroShineFrames
	if subtitleFrames > total {
		total = subtitleFrames
	}
	return total
}

// RenderLogoIntro renders the banner mid-animation, in the exact same
// layout slot RenderLogo occupies once it's done — the rest of the screen
// (outline, footer) is already fully laid out underneath it the whole time.
func RenderLogoIntro(width int, subtitle string, frame int) []string {
	runes := []rune(subtitle)
	shown := frame * IntroCharsPerTick
	if shown > len(runes) {
		shown = len(runes)
	}
	sub := StyleMuted.Render(string(runes[:shown]))
	return []string{centerText(introShineTitle(frame), width), centerText(sub, width)}
}

func introShineTitle(frame int) string {
	const title = "QUESTS"
	sweepPos := float64(frame)/float64(IntroShineFrames)*float64(len(title)+4) - 2

	var b strings.Builder
	for i, ch := range title {
		style := StyleTitle
		dist := float64(i) - sweepPos
		if dist < 0 {
			dist = -dist
		}
		if frame < IntroShineFrames && dist < 1.2 {
			style = lipgloss.NewStyle().Bold(true).Foreground(introShineHighlight)
		}
		b.WriteString(style.Render(string(ch)))
	}
	return b.String()
}

// CenterText centers s within width (ANSI-aware).
func CenterText(s string, width int) string { return centerText(s, width) }

func centerText(s string, width int) string {
	w := lipgloss.Width(s)
	pad := (width - w) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + s
}
