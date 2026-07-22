package app

import (
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// LaunchDarkly "runes" — feature flags you're watching. A rune's live rollout
// state is read via `ldcli` (see the launchdarkly reference): `on` + no rules
// serves the fallthrough variation ("on"); `on` + rules is a targeted/partial
// rollout ("partial"); `on: false` serves the off variation ("off"). Runes
// attach to a quest (Quest.Runes) and/or sit in the Tavern's watched list
// (Store.Runes); both are polled on the sync loop.

// RuneStatus is a flag's resolved state in one environment.
type RuneStatus struct {
	Key    string // flag key, e.g. "scanneros_scan_stream_sync"
	Name   string // human name
	State  string // "on" | "partial" | "off"
	Served string // the served value ("true"/"false"/…) or rule summary
	Env    string // environment key it was read in
}

// ldFlagsResponse is the slice of `ldcli flags list --summary 0` we read.
type ldFlagsResponse struct {
	Items []struct {
		Key        string `json:"key"`
		Name       string `json:"name"`
		Variations []struct {
			Value interface{} `json:"value"`
		} `json:"variations"`
		Environments map[string]struct {
			On           bool `json:"on"`
			OffVariation *int `json:"offVariation"`
			Fallthrough  struct {
				Variation *int `json:"variation"`
			} `json:"fallthrough"`
			Rules []struct{} `json:"rules"`
		} `json:"environments"`
	} `json:"items"`
}

// fetchRuneStatus looks up one flag's state in one environment via ldcli.
// ok is false when ldcli fails or the flag key isn't found.
func fetchRuneStatus(project, env, key string) (RuneStatus, bool) {
	out, err := runCmd("ldcli", "flags", "list", "--project", project,
		"--filter", "query:"+key, "--env", env, "--summary", "0")
	if err != nil {
		return RuneStatus{}, false
	}
	var resp ldFlagsResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return RuneStatus{}, false
	}
	// The filter is a substring match; take the exact-key item.
	for _, f := range resp.Items {
		if f.Key != key {
			continue
		}
		e, ok := f.Environments[env]
		if !ok {
			return RuneStatus{}, false
		}
		val := func(idx *int) string {
			if idx == nil || *idx < 0 || *idx >= len(f.Variations) {
				return ""
			}
			return fmt.Sprintf("%v", f.Variations[*idx].Value)
		}
		st := RuneStatus{Key: key, Name: f.Name, Env: env}
		switch {
		case !e.On:
			st.State, st.Served = "off", val(e.OffVariation)
		case len(e.Rules) > 0:
			st.State, st.Served = "partial", fmt.Sprintf("%d rule(s)", len(e.Rules))
		default:
			st.State, st.Served = "on", val(e.Fallthrough.Variation)
		}
		return st, true
	}
	return RuneStatus{}, false
}

// collectRuneKeys is every distinct flag key to sync — the Tavern's watched
// list plus every quest's attached runes.
func (m *Model) collectRuneKeys() []string {
	seen := map[string]bool{}
	var keys []string
	add := func(k string) {
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, k := range m.store.Runes {
		add(k)
	}
	for i := range m.store.Quests {
		for _, k := range m.store.Quests[i].Runes {
			add(k)
		}
	}
	return keys
}

// runeState is the display state for a flag key from cache, or "" (unknown /
// not yet synced).
func (m *Model) runeState(key string) string {
	if st, ok := m.runeStatus[key]; ok {
		return st.State
	}
	return ""
}

// runeGlyph is the state-colored icon for a rune: a lit circle when on, a
// half circle for a partial/targeted rollout, a hollow circle when off, and
// the muted fetching dot before its first sync.
func (m *Model) runeGlyph(key string) string {
	switch m.runeState(key) {
	case "on":
		return lipgloss.NewStyle().Foreground(ui.ColorHeading).Render(ui.GlyphRuneOn)
	case "partial":
		return lipgloss.NewStyle().Foreground(ui.ColorRunning).Render(ui.GlyphRunePartial)
	case "off":
		return ui.StyleMuted.Render(ui.GlyphRuneOff)
	default:
		return m.pulseStyle().Render(ui.GlyphFetching)
	}
}

// runeWord is the status word shown beside the icon.
func (m *Model) runeWord(key string) string {
	switch m.runeState(key) {
	case "on":
		return "on"
	case "partial":
		if st, ok := m.runeStatus[key]; ok {
			return st.Served // "N rule(s)"
		}
		return "partial"
	case "off":
		return "off"
	default:
		return "fetching…"
	}
}
