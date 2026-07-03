// Package config loads ~/.config/quests/config.toml — every setting is optional
// and falls back to the built-in default, so a missing or partial file is
// always fine. A fully commented sample is written on first run so the
// available settings are discoverable without reading docs.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Behavior Behavior `toml:"behavior"`
	Colors   Colors   `toml:"colors"`
	Icons    Icons    `toml:"icons"`
	Keys     Keys     `toml:"keys"`
}

type Behavior struct {
	// DoneToBottom sorts completed quests to the bottom of their campaign
	// instead of leaving them where they were.
	DoneToBottom bool `toml:"done_to_bottom"`
	// MainToTop floats main quests to the top of their campaign.
	MainToTop bool `toml:"main_to_top"`
	// PriorityToTop floats important (priority) quests to the top —
	// outranking MainToTop when both are on.
	PriorityToTop bool `toml:"priority_to_top"`
	// QuestboardCollapsed starts the Questboard section collapsed.
	QuestboardCollapsed bool `toml:"questboard_collapsed"`
	// VaultCollapsed starts the Vault section collapsed.
	VaultCollapsed bool `toml:"vault_collapsed"`
	// ShowHints shows the inline action hints ("→ open (tab)"); toggleable
	// at runtime either way.
	ShowHints bool `toml:"show_hints"`
	// Intro plays the startup logo animation.
	Intro bool `toml:"intro"`
	// Greeting fixes the subtitle under the logo; empty picks a random
	// tavern greeting each launch.
	Greeting string `toml:"greeting"`
	// Backups writes a daily copy of data.json into the backups/ folder.
	Backups bool `toml:"backups"`
	// BackupKeep is how many daily backups to retain.
	BackupKeep int `toml:"backup_keep"`
}

// Colors are hex values ("#E2B714"); each has a light- and dark-terminal
// variant.
type Colors struct {
	MainLight      string `toml:"main_light"`
	MainDark       string `toml:"main_dark"`
	SideLight      string `toml:"side_light"`
	SideDark       string `toml:"side_dark"`
	HeadingLight   string `toml:"heading_light"`
	HeadingDark    string `toml:"heading_dark"`
	ImportantLight string `toml:"important_light"`
	ImportantDark  string `toml:"important_dark"`
}

type Icons struct {
	QuestOpen   string `toml:"quest_open"`
	QuestActive string `toml:"quest_active"`
	QuestDone   string `toml:"quest_done"`
	NoticeMain  string `toml:"notice_main"`
	NoticeSide  string `toml:"notice_side"`
	Important   string `toml:"important"`
	Expanded    string `toml:"expanded"`
	Collapsed   string `toml:"collapsed"`
}

// Keys rebind the Ctrl/F-key shortcuts, in bubbletea key syntax ("ctrl+d",
// "f1"). The structural keys (arrows, Tab, Enter, Backspace, Esc, Ctrl+C)
// are fixed.
type Keys struct {
	ToggleActive    string `toml:"toggle_active"`
	ToggleDone      string `toml:"toggle_done"`
	ToggleImportant string `toml:"toggle_important"`
	ToggleVault     string `toml:"toggle_vault"`
	ToggleType      string `toml:"toggle_type"`
	MoveCampaign    string `toml:"move_to_campaign"`
	Delete          string `toml:"delete"`
	Search          string `toml:"search"`
	Help            string `toml:"help"`
	ToggleHints     string `toml:"toggle_hints"`
}

func Default() Config {
	return Config{
		Behavior: Behavior{
			DoneToBottom:        true,
			MainToTop:           true,
			PriorityToTop:       true,
			QuestboardCollapsed: true,
			VaultCollapsed:      true,
			ShowHints:           true,
			Intro:               true,
			Greeting:            "",
			Backups:             true,
			BackupKeep:          14,
		},
		Colors: Colors{
			MainLight:      "#DF8E1D",
			MainDark:       "#E2B714",
			SideLight:      "#1E66F5",
			SideDark:       "#89B4FA",
			HeadingLight:   "#40A02B",
			HeadingDark:    "#A6E3A1",
			ImportantLight: "#D20F39",
			ImportantDark:  "#F38BA8",
		},
		Icons: Icons{
			QuestOpen:   "◇",
			QuestActive: "⬖",
			QuestDone:   "◆",
			NoticeMain:  "!",
			NoticeSide:  "?",
			Important:   "↑",
			Expanded:    "▾",
			Collapsed:   "▸",
		},
		Keys: Keys{
			ToggleActive:    "ctrl+a",
			ToggleDone:      "ctrl+d",
			ToggleImportant: "ctrl+p",
			ToggleVault:     "ctrl+v",
			ToggleType:      "ctrl+t",
			MoveCampaign:    "ctrl+o",
			Delete:          "ctrl+x",
			Search:          "ctrl+f",
			Help:            "f1",
			ToggleHints:     "ctrl+k",
		},
	}
}

// Path returns dir/config.toml.
func Path(dir string) string {
	return filepath.Join(dir, "config.toml")
}

// Load reads the config at path over the defaults; a missing file writes
// the commented sample first (all defaults) and returns defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = WriteSample(path) // best-effort; the defaults work regardless
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Default(), fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// WriteSample writes the fully-commented default config to path, creating
// the parent directory as needed. It overwrites unconditionally — callers
// that shouldn't clobber an existing file check for it first (see the
// --init-config handler).
func WriteSample(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sampleConfig), 0o644)
}

const sampleConfig = `# Quests configuration — every setting is optional; delete or comment out
# anything to fall back to the built-in default (the values below ARE the
# defaults).

[behavior]
# Quest ordering within a campaign. On by default; set any to false to keep
# the manual order you arrange them in (Shift+↑/↓) for that dimension. They
# combine; when more than one is on, priority_to_top outranks main_to_top,
# and done_to_bottom wins over both (a finished quest still sinks to the
# bottom).
done_to_bottom = true    # sink completed quests to the bottom
main_to_top = true       # float main quests to the top
priority_to_top = true   # float important (priority) quests to the top
# Start the Questboard and Vault sections collapsed.
questboard_collapsed = true
vault_collapsed = true
# Show the inline action hints ("→ open (tab)"); toggleable at runtime too.
show_hints = true
# Play the startup logo animation.
intro = true
# Fix the subtitle under the logo; leave empty for a random tavern greeting.
greeting = ""
# Write a daily copy of data.json into the backups/ folder next to this
# file, keeping the most recent backup_keep days.
backups = true
backup_keep = 14

[colors]
# Hex colors; *_light applies on light terminal themes, *_dark on dark.
# main: main quests (gold) · side: side quests (blue) · heading: "# " lines
# · important: the priority arrow (red).
main_light = "#DF8E1D"
main_dark = "#E2B714"
side_light = "#1E66F5"
side_dark = "#89B4FA"
heading_light = "#40A02B"
heading_dark = "#A6E3A1"
important_light = "#D20F39"
important_dark = "#F38BA8"

[icons]
# Single-character glyphs. quest_* is the shape by progress; notice_* marks
# untriaged Questboard quests; important is the priority marker shown left
# of a flagged quest; expanded/collapsed are the fold carets.
quest_open = "◇"
quest_active = "⬖"
quest_done = "◆"
notice_main = "!"
notice_side = "?"
important = "↑"
expanded = "▾"
collapsed = "▸"

[keys]
# Rebind shortcuts using bubbletea key syntax ("ctrl+d", "f1"). Arrows,
# Tab, Enter, Backspace, Esc and Ctrl+C are structural and can't move.
# Avoid ctrl+m / ctrl+i / ctrl+h — terminals send those as Enter / Tab /
# Backspace, so they're indistinguishable and can't be bound here. Chords
# that a row's text editor also uses (ctrl+a/e/w/u/k) still work as
# commands on the outline — you lose them for text editing there, but Home/
# End/arrows cover it (e.g. ctrl+a is "toggle active"; Home jumps to line
# start).
toggle_active = "ctrl+a"
toggle_done = "ctrl+d"
toggle_important = "ctrl+p"
toggle_vault = "ctrl+v"
toggle_type = "ctrl+t"
move_to_campaign = "ctrl+o"
delete = "ctrl+x"
search = "ctrl+f"
help = "f1"
toggle_hints = "ctrl+k"
`
