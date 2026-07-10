package ui

import "github.com/charmbracelet/lipgloss"

// Glyph set: the entire "branded like a game, but not too much" look lives
// here as plain Unicode text icons, so it's easy to retune in one place.
//
// A quest normally shows a single diamond glyph whose SHAPE carries its
// progress (open → active → done) and whose COLOR carries its type (gold
// for main, blue for side, via StyleMain/StyleSide) — one icon doing both
// jobs instead of a redundant type-glyph + todo-checkbox pair. A quest still
// sitting on the Questboard (no campaign yet) instead shows the classic
// RPG "quest available" notice mark — "!" for main, "?" for side — since
// there's no progress to track until it's picked up. An objective inside a
// quest's detail view reuses the diamond open/done shapes, just muted
// instead of colored — objectives don't have a main/side axis, so that
// color is reserved for actual quests. Every glyph here is a single
// narrow-width character from the same handful of Unicode blocks, chosen so
// they render at one consistent column width next to each other.
var (
	GlyphQuestOpen   = "◇" // hollow diamond — not started
	GlyphQuestActive = "⬖" // left-half-filled diamond — underway
	GlyphQuestDone   = "◆" // fully filled diamond — completed

	GlyphNoticeMain = "!" // Questboard notice — main quest available
	GlyphNoticeSide = "?" // Questboard notice — side quest available

	GlyphImportant   = "↑" // medium/high priority marker, shown left of a quest
	GlyphPriorityLow = "↓" // low-priority (deprioritized) marker

	GlyphExpanded  = "▾"
	GlyphCollapsed = "▸"
	GlyphCursor    = "› "

	// Integration status glyphs (see internal/app/sync.go rendering). Each is
	// a single narrow char, matching the glyph discipline above. Jira status
	// is a filling circle (empty → half → full); a PR shows a check/cross for
	// CI and a dotted circle while it's still running; a code whose sync
	// hasn't landed yet shows the muted loading dot.
	GlyphJiraTodo       = "○" // Jira: to do (muted)
	GlyphJiraInProgress = "◑" // Jira: in progress (gold)
	GlyphJiraDone       = "●" // Jira: done (green)
	GlyphPRSuccess      = "✓" // PR CI: success (green)
	GlyphPRError        = "✗" // PR CI: error/failure (red)
	GlyphPRRunning      = "◌" // PR CI: running (amber)
	GlyphPRMerged       = "◆" // PR merged (mauve) — outranks CI state
	GlyphPRClosed       = "⊘" // PR closed unmerged (muted)
	GlyphFetching       = "◌" // code linked but not yet synced — "fetching" (amber)
	GlyphLoading        = "·" // legacy muted loading dot (kept for compatibility)

	// Graphite-style stack markers, drawn in a left gutter before a PR that
	// belongs to a stack (2+ linked PRs): every PR but the last uses the tee,
	// the last uses the corner — a compact file-tree look. A lone PR gets no
	// marker. Both render muted (see sync.go focusCodeLines).
	GlyphStackBranchMid = "├" // a PR with another below it in the stack
	GlyphStackBranchEnd = "└" // the last (deepest) PR in the stack

	// Claude-agent status sparks (see internal/app/agents.go): a filled spark
	// while working (amber) or done (green), a hollow spark while idle or when
	// the pinned worktree currently has no tracked session.
	GlyphAgentWorking = "✦" // agent working (amber)
	GlyphAgentDone    = "✦" // agent finished (green)
	GlyphAgentIdle    = "✧" // agent idle (muted)
	GlyphAgentNone    = "✧" // pinned worktree, no session (muted)
)

// IconSet overrides the glyphs above from user config — empty fields keep
// the default.
type IconSet struct {
	QuestOpen, QuestActive, QuestDone string
	NoticeMain, NoticeSide            string
	Important, PriorityLow            string
	Expanded, Collapsed               string
}

func ApplyIcons(ic IconSet) {
	setIf(&GlyphQuestOpen, ic.QuestOpen)
	setIf(&GlyphQuestActive, ic.QuestActive)
	setIf(&GlyphQuestDone, ic.QuestDone)
	setIf(&GlyphNoticeMain, ic.NoticeMain)
	setIf(&GlyphNoticeSide, ic.NoticeSide)
	setIf(&GlyphImportant, ic.Important)
	setIf(&GlyphPriorityLow, ic.PriorityLow)
	setIf(&GlyphExpanded, ic.Expanded)
	setIf(&GlyphCollapsed, ic.Collapsed)
}

func setIf(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

// Plain body/title text deliberately never sets an explicit Foreground: a
// fixed "looks great on dark bg" color is unreadable on a light-themed
// terminal, which makes the whole UI look blank. Leaving it unset inherits
// the terminal's own foreground, which is correct by construction on any
// theme. "Muted" uses Faint (a relative dimming of whatever that foreground
// is) for the same reason. Only true accent hues (which need a specific
// color to convey meaning) use an AdaptiveColor pair (Catppuccin Mocha for
// dark terminals, Latte for light).
var (
	ColorAccent         = lipgloss.AdaptiveColor{Light: "#DF8E1D", Dark: "#E2B714"}
	ColorSide           = lipgloss.AdaptiveColor{Light: "#1E66F5", Dark: "#89B4FA"}
	ColorHeading        = lipgloss.AdaptiveColor{Light: "#40A02B", Dark: "#A6E3A1"}
	ColorImportant      = lipgloss.AdaptiveColor{Light: "#D20F39", Dark: "#F38BA8"} // high priority (red)
	ColorPriorityMedium = lipgloss.AdaptiveColor{Light: "#DF8E1D", Dark: "#F9E2AF"} // medium priority (yellow)
	ColorRunning        = lipgloss.AdaptiveColor{Light: "#FE640B", Dark: "#FAB387"} // integration "running" state (amber)
	ColorMerged         = lipgloss.AdaptiveColor{Light: "#8839EF", Dark: "#CBA6F7"} // merged PR (mauve)
	ColorSelected       = lipgloss.AdaptiveColor{Light: "#CCD0DA", Dark: "#313244"}

	StyleTitle          = lipgloss.NewStyle().Bold(true)
	StyleMuted          = lipgloss.NewStyle().Faint(true)
	StyleDone           = lipgloss.NewStyle().Faint(true)
	StyleMain           = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleSide           = lipgloss.NewStyle().Foreground(ColorSide)
	StyleCursor         = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleImportant      = lipgloss.NewStyle().Bold(true).Foreground(ColorImportant)      // high priority
	StylePriorityMedium = lipgloss.NewStyle().Bold(true).Foreground(ColorPriorityMedium) // medium priority
	StyleRunning        = lipgloss.NewStyle().Foreground(ColorRunning)                   // integration "running" state
	StyleMerged         = lipgloss.NewStyle().Foreground(ColorMerged)                    // merged PR

	// Selected rows (used by the project-picker modal's list) set both fg
	// and bg explicitly as a self-consistent pair, so contrast holds
	// regardless of the ambient terminal background.
	StyleSelectedRow = lipgloss.NewStyle().Background(ColorSelected).Bold(true)

	StyleHeading       = lipgloss.NewStyle().Bold(true).Foreground(ColorHeading)
	StyleSectionHeader = lipgloss.NewStyle().Bold(true)
	StyleFooter        = lipgloss.NewStyle().Faint(true).Padding(0, 1)
)

// Theme overrides the accent colors from user config — empty fields keep
// the default. Call before the program starts; the derived styles are
// rebuilt here.
type Theme struct {
	MainLight, MainDark                     string
	SideLight, SideDark                     string
	HeadingLight, HeadingDark               string
	ImportantLight, ImportantDark           string
	PriorityMediumLight, PriorityMediumDark string
}

func ApplyTheme(t Theme) {
	setIf(&ColorAccent.Light, t.MainLight)
	setIf(&ColorAccent.Dark, t.MainDark)
	setIf(&ColorSide.Light, t.SideLight)
	setIf(&ColorSide.Dark, t.SideDark)
	setIf(&ColorHeading.Light, t.HeadingLight)
	setIf(&ColorHeading.Dark, t.HeadingDark)
	setIf(&ColorImportant.Light, t.ImportantLight)
	setIf(&ColorImportant.Dark, t.ImportantDark)
	setIf(&ColorPriorityMedium.Light, t.PriorityMediumLight)
	setIf(&ColorPriorityMedium.Dark, t.PriorityMediumDark)

	StyleMain = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleSide = lipgloss.NewStyle().Foreground(ColorSide)
	StyleCursor = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleHeading = lipgloss.NewStyle().Bold(true).Foreground(ColorHeading)
	StyleImportant = lipgloss.NewStyle().Bold(true).Foreground(ColorImportant)
	StylePriorityMedium = lipgloss.NewStyle().Bold(true).Foreground(ColorPriorityMedium)
}

// Sort-behavior flags, all set from config (see questPriority). By default
// all off, so quests keep their manual order. DoneToBottom sinks completed
// quests; MoveMainToTop floats main quests; MovePriorityToTop floats
// important quests (and outranks MoveMainToTop when both are on).
var (
	DoneToBottom        bool
	MoveMainToTop       bool
	MovePriorityToTop   bool
	LowPriorityToBottom bool
)
