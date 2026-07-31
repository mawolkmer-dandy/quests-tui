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
	Layout   Layout   `toml:"layout"`
	Keys     Keys     `toml:"keys"`
	Sound    Sound    `toml:"sound"`
}

// Sound configures the per-event sound clips (macOS `afplay`). Empty paths
// fall back to bundled clips shipped inside the binary, so enabling it works
// out of the box; set a path to use your own file for any event.
type Sound struct {
	// Enabled turns all sound on/off.
	Enabled bool `toml:"enabled"`
	// Each field overrides one event's clip (empty = bundled default).
	QuestDone     string `toml:"quest_done"`     // a quest marked done
	ObjectiveDone string `toml:"objective_done"` // an objective checked off
	QuestActive   string `toml:"quest_active"`   // a quest taken up (active)
	EnterTavern   string `toml:"enter_tavern"`   // entering the Tavern (also on launch)
	EnterWilds    string `toml:"enter_wilds"`    // setting out to the Wilds
	AddConnection string `toml:"add_connection"` // a Jira/PR/rune/agent linked to a quest
	OpenNPC       string `toml:"open_npc"`       // the agent picker opens
}

type Behavior struct {
	// DoneToBottom sorts completed quests to the bottom of their campaign
	// instead of leaving them where they were.
	DoneToBottom bool `toml:"done_to_bottom"`
	// MainToTop floats main quests to the top of their campaign.
	MainToTop bool `toml:"main_to_top"`
	// PriorityToTop floats medium/high priority quests to the top —
	// outranking MainToTop when both are on.
	PriorityToTop bool `toml:"priority_to_top"`
	// LowPriorityToBottom sinks low-priority quests to the bottom of their
	// list, just above completed ones.
	LowPriorityToBottom bool `toml:"low_priority_to_bottom"`
	// ShowHints shows the inline action hints ("→ open (tab)"); toggleable
	// at runtime either way.
	ShowHints bool `toml:"show_hints"`
	// Animations plays the environment-change animation (startup, Tavern⇄
	// Wilds, and filter changes). Set false to switch instantly.
	Animations bool `toml:"animations"`
	// Greeting fixes the subtitle under the logo; empty picks a random
	// tavern greeting each launch.
	Greeting string `toml:"greeting"`
	// Backups writes a daily copy of data.json into the backups/ folder.
	Backups bool `toml:"backups"`
	// BackupKeep is how many daily backups to retain.
	BackupKeep int `toml:"backup_keep"`
	// IntegrationsEnabled turns on Jira/GitHub-PR linking: paste a URL into a
	// quest's body to link it, and its status refreshes in the background.
	// Requires `gh` and `acli` authenticated locally.
	IntegrationsEnabled bool `toml:"integrations_enabled"`
	// SyncIntervalSecs is how often (seconds) linked PR/Jira statuses refresh;
	// a 15s floor is enforced when applied.
	SyncIntervalSecs int `toml:"sync_interval_secs"`
	// JiraBaseURL is the Jira instance base for building clickable browse
	// links from an issue key.
	JiraBaseURL string `toml:"jira_base_url"`
	// LDProject is the LaunchDarkly project key that watched runes (feature
	// flags) are looked up in via `ldcli`.
	LDProject string `toml:"ld_project"`
	// LDEnv is the LaunchDarkly environment whose targeting state a rune shows.
	LDEnv string `toml:"ld_env"`
}

// Layout persists the two-column Tavern's user-resized proportions and
// section collapse state across restarts — grab a border to resize, click a
// chevron to collapse; both are written automatically as they change, you
// normally never edit this by hand.
type Layout struct {
	// RailWidthRatio is the left rail's fraction of the Tavern's content
	// width; the campaigns column takes the remainder (minus the fixed gap).
	RailWidthRatio float64 `toml:"rail_width_ratio"`
	// RailBoxRatios are the three rail boxes' (Questboard, Runes, Vault)
	// relative height weights, in that order.
	RailBoxRatios [3]float64 `toml:"rail_box_ratios"`
	// CollapsedSections lists which of "inbox" (Questboard) / "runes" /
	// "someday" (Vault) are currently collapsed. Absent = expanded.
	CollapsedSections []string `toml:"collapsed_sections"`
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
			LowPriorityToBottom: true,
			ShowHints:           true,
			Animations:          true,
			Greeting:            "",
			Backups:             true,
			BackupKeep:          14,
			IntegrationsEnabled: true,
			SyncIntervalSecs:    60,
			JiraBaseURL:         "https://meetdandy.atlassian.net",
			LDProject:           "default",
			LDEnv:               "production",
		},
		Layout: Layout{
			RailWidthRatio: 0.34,
			RailBoxRatios:  [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3},
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
		Sound: Sound{
			Enabled: true, // bundled sounds play out of the box; set false to silence
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

// Save atomically writes cfg to path as TOML (temp file + rename, same
// crash-safety pattern as store.Save). Unlike WriteSample (a fixed commented
// template for a fresh install), this serializes the live Config — currently
// only a resize-drag release calls this. Note: BurntSushi/toml has no
// round-trip-with-comments support, so this replaces the WHOLE file,
// including any hand-edited comments elsewhere in it — an accepted tradeoff
// given the file is already machine-writable in one place (WriteSample) and
// resizing is the only thing that ever calls Save.
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.toml.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
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
done_to_bottom = true          # sink completed quests to the bottom
main_to_top = true             # float main quests to the top
priority_to_top = true         # float medium/high priority quests to the top
low_priority_to_bottom = true  # sink low-priority quests (just above done)
# Show the inline action hints ("→ open (tab)"); toggleable at runtime too.
show_hints = true
# Play the environment-change animation (startup, Tavern⇄Wilds, filtering).
# Set false to switch instantly.
animations = true
# Fix the subtitle under the logo; leave empty for a random tavern greeting.
greeting = ""
# Write a daily copy of data.json into the backups/ folder next to this
# file, keeping the most recent backup_keep days.
backups = true
backup_keep = 14
# Link Jira issues and GitHub PRs by pasting their URL into a quest's body:
# the code (e.g. EPDCHAIR-5713 / #47477) then shows its live status, refreshed
# every sync_interval_secs (minimum 15). Requires gh and acli authenticated
# locally (gh auth login; acli jira auth login). jira_base_url builds the
# clickable browse link from an issue key.
integrations_enabled = true
sync_interval_secs = 60
jira_base_url = "https://meetdandy.atlassian.net"

[layout]
# The two-column Tavern's rail/campaigns width split, the three rail boxes'
# (Questboard/Runes/Vault) height split, and which of them are collapsed —
# all written here automatically as you drag a border or click a chevron.
# Note that this rewrites this whole file, so any comments you've added
# elsewhere won't survive a change.
rail_width_ratio = 0.34
rail_box_ratios = [0.333, 0.333, 0.333]
collapsed_sections = []

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

[sound]
# Per-event sound clips (macOS, via afplay). On by default using bundled
# clips; set enabled = false to silence. Point any event at your own file
# (mp3/wav/aiff/m4a) to override it — empty = the bundled clip.
enabled = true
quest_done = ""      # a quest marked done
objective_done = ""  # an objective checked off
quest_active = ""    # a quest taken up (active)
enter_tavern = ""    # entering the Tavern (also plays on launch)
enter_wilds = ""     # setting out to the Wilds
add_connection = ""  # a Jira/PR/rune/agent linked to a quest
open_npc = ""        # the agent picker opens
`
