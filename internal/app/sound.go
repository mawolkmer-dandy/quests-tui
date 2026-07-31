package app

import (
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/config"
)

// Sound plays a short clip at key moments so the app feels alive. Each event
// has a bundled default (embedded below) that a user can override by pointing
// the matching [sound] config path at their own file. Fire-and-forget via
// `afplay` (macOS) — audio never blocks the UI, and it's a no-op elsewhere,
// when disabled, or when a file is missing.

//go:embed sounds/*.mp3
var soundFS embed.FS

type soundEvent int

const (
	sndQuestDone soundEvent = iota
	sndObjectiveDone
	sndQuestActive
	sndEnterTavern
	sndEnterWilds
	sndAddConnection
	sndOpenNPC
)

// soundFile is the bundled clip for each event (under sounds/, embedded).
var soundFile = map[soundEvent]string{
	sndQuestDone:     "mark_quest_as_done.mp3",
	sndObjectiveDone: "mark_objective_as_done.mp3",
	sndQuestActive:   "mark_quest_as_active.mp3",
	sndEnterTavern:   "enter_tavern.mp3",
	sndEnterWilds:    "enter_wilds.mp3",
	sndAddConnection: "add_connection_to_quest.mp3",
	sndOpenNPC:       "open_npc_selection.mp3",
}

// override returns the user's configured path for an event ("" = use bundled).
func (m *Model) soundOverride(e soundEvent) string {
	switch e {
	case sndQuestDone:
		return m.soundCfg.QuestDone
	case sndObjectiveDone:
		return m.soundCfg.ObjectiveDone
	case sndQuestActive:
		return m.soundCfg.QuestActive
	case sndEnterTavern:
		return m.soundCfg.EnterTavern
	case sndEnterWilds:
		return m.soundCfg.EnterWilds
	case sndAddConnection:
		return m.soundCfg.AddConnection
	case sndOpenNPC:
		return m.soundCfg.OpenNPC
	}
	return ""
}

// playSound plays the clip for an event: the user's override if set, else the
// bundled default. Returns nil (no-op) when sound is off, unsupported, or the
// resolved file is missing.
func (m *Model) playSound(e soundEvent) tea.Cmd {
	if !m.soundCfg.Enabled || runtime.GOOS != "darwin" {
		return nil
	}
	path := m.soundOverride(e)
	if path == "" {
		path = extractBundledSound(soundFile[e])
	}
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil // configured file gone — stay silent rather than erroring
	}
	return func() tea.Msg {
		_ = exec.Command("afplay", path).Start() // detached; we don't wait
		return nil
	}
}

// toggleMute flips all sound on/off and persists it to config.toml so the
// choice survives restarts, with a brief on-screen confirmation.
func (m *Model) toggleMute() tea.Cmd {
	m.soundCfg.Enabled = !m.soundCfg.Enabled
	m.saveSoundConfig()
	label := "🔊 sound on"
	if !m.soundCfg.Enabled {
		label = "🔇 sound muted"
	}
	return m.showWarning(m.cursor, label)
}

// saveSoundConfig persists the current mute state to config.toml (best-effort,
// same load-mutate-save pattern as the layout persistence).
func (m *Model) saveSoundConfig() {
	if m.cfgPath == "" {
		return
	}
	cfg, err := config.Load(m.cfgPath)
	if err != nil {
		return
	}
	cfg.Sound = m.soundCfg
	_ = config.Save(m.cfgPath, cfg)
}

// extractBundledSound writes an embedded clip to the user cache dir once (so
// afplay has a real path regardless of where the binary is installed) and
// returns it, or "" if the cache dir can't be used.
func extractBundledSound(name string) string {
	if name == "" {
		return ""
	}
	data, err := soundFS.ReadFile("sounds/" + name)
	if err != nil {
		return ""
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	sdir := filepath.Join(dir, "quests", "sounds")
	if err := os.MkdirAll(sdir, 0o755); err != nil {
		return ""
	}
	p := filepath.Join(sdir, name)
	if fi, err := os.Stat(p); err != nil || fi.Size() != int64(len(data)) {
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return ""
		}
	}
	return p
}
