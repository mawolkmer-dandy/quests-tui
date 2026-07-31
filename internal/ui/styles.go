package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Glyph set: the entire "branded like a game, but not too much" look lives
// here — mostly plain Unicode text icons, with real Nerd Font glyphs where
// they add real semantic/brand value (connections, integration status, agent
// state). Nerd Font glyphs are written as explicit \u/\U escapes (not raw
// characters) so the source stays readable without the font installed.
//
// A quest normally shows a single rhombus glyph whose SHAPE carries its
// progress (open → active → done — hollow, split/half, full solid, one
// consistent Nerd Font family) and whose COLOR carries its type (gold for
// main, blue for side, via StyleMain/StyleSide) — one icon doing both jobs
// instead of a redundant type-glyph + todo-checkbox pair. A quest still
// sitting on the Questboard (no campaign yet) instead shows the classic
// RPG "quest available" notice mark — "!" for main, "?" for side (the
// rhombus-framed alert/help NF glyphs render too small/faint at normal text
// size, so these stay plain punctuation) — since there's no progress to
// track until it's picked up. An objective inside a quest's detail view
// reuses the rhombus open/done shapes, just muted instead of colored —
// objectives don't have a main/side axis, so that color is reserved for
// actual quests. Every glyph here is a single narrow-width character,
// chosen so they render at one consistent column width next to each other.
var (
	GlyphQuestOpen   = "\U000f070c" // nf-md-rhombus_outline — hollow, not started
	GlyphQuestActive = "\U000f02b9" // nf-md-google_nearby — underway
	GlyphQuestDone   = "\U000f070b" // nf-md-rhombus — full solid, completed

	GlyphNoticeMain = "!" // Questboard notice — main quest available
	GlyphNoticeSide = "?" // Questboard notice — side quest available

	GlyphImportant   = "\U000f0737" // nf-md-arrow_up_bold — medium/high priority marker, shown left of a quest
	GlyphPriorityLow = "\U000f072e" // nf-md-arrow_down_bold — low-priority (deprioritized) marker

	GlyphExpanded  = "▾"
	GlyphCollapsed = "▸"
	GlyphCursor    = "› "

	// Tavern ornaments (two-column layout). Campaign banners get a small
	// fleur/flourish; the Vault is framed with green foliage sprigs so the old
	// rusted strongbox looks overgrown. All are narrow dingbats (not emoji) so
	// they hold one column and don't force a wide fallback.
	GlyphOrnament  = "❖" // campaign banner flourish
	GlyphFlourishL = "❦" // left banner sprig
	GlyphFlourishR = "❧" // right banner sprig
	GlyphFoliage   = "❧" // vault foliage sprig
	GlyphVine      = "✦" // small accent star

	// GlyphArchived marks a retired campaign sitting inline in the Vault's day
	// timeline.
	GlyphArchived = "\U000f003c" // nf-md-archive

	// Integration status glyphs (see internal/app/sync.go rendering). Jira
	// status is a filling circle (empty → half → full); a PR uses real
	// GitHub/Octicon-style glyphs for its CI/merge state; a code whose sync
	// hasn't landed yet shows a spinning sync glyph.
	GlyphJiraTodo       = "○"          // Jira: to do (muted)
	GlyphJiraInProgress = "◑"          // Jira: in progress (gold)
	GlyphJiraDone       = "●"          // Jira: done (green)
	GlyphPRSuccess      = "\uf42e"     // nf-oct-check — PR CI: success (green)
	GlyphPRError        = "\uf530"     // nf-oct-x_circle_fill — PR CI: error/failure (red)
	GlyphPRRunning      = "\U000f0996" // nf-md-progress_clock — PR CI: running (amber, pulsing)
	GlyphPRMerged       = "\uf419"     // nf-oct-git_merge — PR merged (mauve) — outranks CI state
	GlyphPRClosed       = "\uf4dc"     // nf-oct-git_pull_request_closed — PR closed unmerged (muted)
	GlyphFetching       = "\U000f04e6" // nf-md-sync — code linked but not yet synced — "fetching" (amber, pulsing)
	GlyphLoading        = "·"          // legacy muted loading dot (kept for compatibility)

	// Graphite-style stack markers, drawn in a left gutter before a PR that
	// belongs to a stack (2+ linked PRs): every PR but the last uses the tee,
	// the last uses the corner — a compact file-tree look. A lone PR gets no
	// marker. Both render muted (see sync.go focusCodeLines).
	GlyphStackBranchMid = "├" // a PR with another below it in the stack
	GlyphStackBranchEnd = "└" // the last (deepest) PR in the stack

	// LaunchDarkly "rune" (feature-flag) status icons (see internal/app/runes.go):
	// a lit circle when the flag is on, a half circle for a targeted/partial
	// rollout, a hollow circle when off.
	GlyphRuneOn      = "●" // flag on (green)
	GlyphRunePartial = "◐" // targeted / partial rollout (amber)
	GlyphRuneOff     = "○" // flag off (muted)

	// Connection type emblems — one glyph per connection kind, shown inline
	// after a quest's title (colored by status) and as each section's header in
	// the detail view. NPC = a pinned Claude agent, Scrolls = Jira specs,
	// Trails = GitHub PRs, Runes = LaunchDarkly flags — real brand/semantic
	// Nerd Font glyphs instead of abstract dingbats.
	GlyphConnNPC    = "\U000f06a9" // nf-md-robot — NPC — a pinned agent
	GlyphConnScroll = "\U000f0303" // nf-md-jira — Scrolls — a Jira spec
	GlyphConnTrail  = "\uf407"     // nf-oct-git_pull_request — Trails — a GitHub PR
	GlyphConnRune   = "\U000f0521" // nf-md-toggle_switch — Runes — a LaunchDarkly flag

	// Claude-agent status icons (see internal/app/agents.go), traffic-light
	// colored: blocked/waiting-for-input is red and demands attention, working
	// is amber, idle/done are a green check, paused is a muted bar.
	GlyphAgentBlocked = "\U000f0005" // nf-md-account_alert — waiting for input / blocked (red)
	GlyphAgentWorking = "\U000f070e" // nf-md-run — actively working (amber)
	GlyphAgentIdle    = "\uf42e"     // nf-oct-check — idle or done (green)
	GlyphAgentPaused  = "\U000f03e5" // nf-md-pause_circle — paused (muted)
	GlyphAgentNone    = "·"          // pinned worktree, no tracked session (muted)
)

// Plain body/title text deliberately never sets an explicit Foreground: a
// fixed "looks great on dark bg" color is unreadable on a light-themed
// terminal, which makes the whole UI look blank. Leaving it unset inherits
// the terminal's own foreground, which is correct by construction on any
// theme. "Muted" uses Faint (a relative dimming of whatever that foreground
// is) for the same reason. Only true accent hues (which need a specific
// color to convey meaning) resolve through Init's light/dark pair (Catppuccin
// Mocha for dark terminals, Latte for light).
var (
	ColorAccent         color.Color
	ColorSide           color.Color
	ColorHeading        color.Color
	ColorImportant      color.Color // high priority (red)
	ColorPriorityMedium color.Color // medium priority (yellow)
	ColorRunning        color.Color // integration "running" state (amber)

	// PulseAmber is a bright→dim→bright amber ramp, cycled by the spinner clock
	// (internal/app) to "pulse" the fetching and CI-running status icons.
	PulseAmber []color.Color

	ColorMerged   color.Color // merged PR (mauve)
	ColorSelected color.Color

	// ColorRust ages the Vault's frame — a weathered iron/rust brown, so the
	// Vault reads like an old strongbox rather than another live section.
	ColorRust color.Color

	// Per-section accent colors (two-column Tavern). Runes are purple, the
	// campaigns list is a neutral gray; Questboard reuses the warm accent and
	// the Vault its rust. Each shows muted normally and full only when its
	// section holds the cursor.
	ColorRune     color.Color
	ColorCampaign color.Color

	StyleTitle = lipgloss.NewStyle().Bold(true)
	// StyleName is a non-bold quest/campaign name (the resting state); only the
	// selected row's name is bold (StyleTitle, via cursorTitleStyle).
	StyleName           = lipgloss.NewStyle()
	StyleMuted          = lipgloss.NewStyle().Faint(true)
	StyleDone           = lipgloss.NewStyle().Faint(true)
	StyleMain           lipgloss.Style
	StyleSide           lipgloss.Style
	StyleCursor         lipgloss.Style
	StyleImportant      lipgloss.Style // high priority
	StylePriorityMedium lipgloss.Style // medium priority
	StyleRunning        lipgloss.Style // integration "running" state
	StyleMerged         lipgloss.Style // merged PR

	// Selected rows (used by the project-picker modal's list) set both fg
	// and bg explicitly as a self-consistent pair, so contrast holds
	// regardless of the ambient terminal background.
	StyleSelectedRow lipgloss.Style

	StyleHeading       lipgloss.Style
	StyleSectionHeader = lipgloss.NewStyle().Bold(true)
	StyleFooter        = lipgloss.NewStyle().Faint(true).Padding(0, 1)

	// Two-column Tavern chrome: the rail box frame (warm accent), the Vault's
	// aged/rusted frame, and the "glow" on a taken-up (active) quest so it
	// stands out as the thing you're currently on.
	StyleRailFrame   lipgloss.Style
	StyleVaultFrame  lipgloss.Style
	StyleActiveQuest lipgloss.Style

	// Ornament styles: warm accent flourishes on campaign banners, green
	// foliage around the Vault.
	StyleOrnament lipgloss.Style
	StyleFoliage  lipgloss.Style
)

// Init resolves every accent color against the terminal's actual background
// and builds the styles derived from them — call once at startup (see
// main.go), before any Style above is used. lipgloss v2 dropped
// AdaptiveColor in favor of this explicit LightDark pattern (no more global
// stdin/stdout detection baked into the color type itself).
func Init(darkBg bool) {
	ld := lipgloss.LightDark(darkBg)

	ColorAccent = ld(lipgloss.Color("#DF8E1D"), lipgloss.Color("#E2B714"))
	ColorSide = ld(lipgloss.Color("#1E66F5"), lipgloss.Color("#89B4FA"))
	ColorHeading = ld(lipgloss.Color("#40A02B"), lipgloss.Color("#A6E3A1"))
	ColorImportant = ld(lipgloss.Color("#D20F39"), lipgloss.Color("#F38BA8"))
	ColorPriorityMedium = ld(lipgloss.Color("#DF8E1D"), lipgloss.Color("#F9E2AF"))
	ColorRunning = ld(lipgloss.Color("#FE640B"), lipgloss.Color("#FAB387"))
	PulseAmber = []color.Color{
		ld(lipgloss.Color("#FE640B"), lipgloss.Color("#FAB387")),
		ld(lipgloss.Color("#F27A32"), lipgloss.Color("#DFA47D")),
		ld(lipgloss.Color("#E69259"), lipgloss.Color("#C49072")),
		ld(lipgloss.Color("#DBAA80"), lipgloss.Color("#A97C66")),
		ld(lipgloss.Color("#E69259"), lipgloss.Color("#C49072")),
		ld(lipgloss.Color("#F27A32"), lipgloss.Color("#DFA47D")),
	}
	ColorMerged = ld(lipgloss.Color("#8839EF"), lipgloss.Color("#CBA6F7"))
	ColorSelected = ld(lipgloss.Color("#CCD0DA"), lipgloss.Color("#313244"))
	ColorRust = ld(lipgloss.Color("#B4703A"), lipgloss.Color("#A9764F"))
	ColorRune = ld(lipgloss.Color("#8839EF"), lipgloss.Color("#CBA6F7"))
	ColorCampaign = ld(lipgloss.Color("#6C6F85"), lipgloss.Color("#9399B2"))
	introShineHighlight = ld(lipgloss.Color("#B8860B"), lipgloss.Color("#FFD54F"))

	StyleMain = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleSide = lipgloss.NewStyle().Foreground(ColorSide)
	StyleCursor = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleImportant = lipgloss.NewStyle().Bold(true).Foreground(ColorImportant)
	StylePriorityMedium = lipgloss.NewStyle().Bold(true).Foreground(ColorPriorityMedium)
	StyleRunning = lipgloss.NewStyle().Foreground(ColorRunning)
	StyleMerged = lipgloss.NewStyle().Foreground(ColorMerged)
	StyleSelectedRow = lipgloss.NewStyle().Background(ColorSelected).Bold(true)
	StyleHeading = lipgloss.NewStyle().Bold(true).Foreground(ColorHeading)
	StyleRailFrame = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleVaultFrame = lipgloss.NewStyle().Faint(true).Foreground(ColorRust)
	StyleActiveQuest = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	StyleOrnament = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleFoliage = lipgloss.NewStyle().Foreground(ColorHeading)
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
