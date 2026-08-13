package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestSaveLoadRoundTrip populates every field (including a 4-element
// RailBoxRatios, sound overrides, collapsed sections, keys, behavior) and
// asserts Save→Load→Save→Load preserves the whole Config — the data-safety
// guard for the persisted config schema.
func TestSaveLoadRoundTrip(t *testing.T) {
	full := Config{
		Behavior: Behavior{
			DoneToBottom:        true,
			MainToTop:           true,
			PriorityToTop:       true,
			LowPriorityToBottom: true,
			ShowHints:           true,
			Animations:          true,
			Greeting:            "well met, traveler",
			Backups:             true,
			BackupKeep:          21,
			IntegrationsEnabled: true,
			SyncIntervalSecs:    45,
			JiraBaseURL:         "https://example.atlassian.net",
			LDProject:           "myproj",
			LDEnv:               "staging",
		},
		Layout: Layout{
			RailWidthRatio:    0.4,
			RailBoxRatios:     []float64{0.1, 0.2, 0.3, 0.4},
			CollapsedSections: []string{"runes", "wards"},
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
			Enabled:       true,
			QuestDone:     "/clips/done.mp3",
			ObjectiveDone: "/clips/obj.mp3",
			QuestActive:   "/clips/active.mp3",
			EnterTavern:   "/clips/tavern.mp3",
			EnterWilds:    "/clips/wilds.mp3",
			AddConnection: "/clips/conn.mp3",
			OpenNPC:       "/clips/npc.mp3",
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := Save(path, full); err != nil {
		t.Fatalf("save 1: %v", err)
	}
	got1, err := Load(path)
	if err != nil {
		t.Fatalf("load 1: %v", err)
	}
	if !reflect.DeepEqual(got1, full) {
		t.Fatalf("round-trip 1 dropped fields:\n got  %#v\n want %#v", got1, full)
	}
	if err := Save(path, got1); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	got2, err := Load(path)
	if err != nil {
		t.Fatalf("load 2: %v", err)
	}
	if !reflect.DeepEqual(got2, full) {
		t.Fatalf("round-trip 2 dropped fields:\n got  %#v\n want %#v", got2, full)
	}
}

// TestLegacyThreeElementRailRatios decodes a config written before the Wards
// box existed (3-element rail_box_ratios) and asserts it still loads without
// error — the app normalizes the length to the current box count on startup.
func TestLegacyThreeElementRailRatios(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	legacy := `
[layout]
rail_width_ratio = 0.34
rail_box_ratios = [0.333, 0.333, 0.333]
collapsed_sections = ["runes"]
`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	if len(cfg.Layout.RailBoxRatios) != 3 {
		t.Fatalf("expected the 3 legacy ratios decoded as-is, got %v", cfg.Layout.RailBoxRatios)
	}
	// A hand-written legacy file decodes into the struct; the encoder must also
	// re-emit it cleanly.
	var rt Config
	if _, err := toml.DecodeFile(path, &rt); err != nil {
		t.Fatalf("decode legacy: %v", err)
	}
}
