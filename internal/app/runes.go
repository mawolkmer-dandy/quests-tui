package app

import (
	"encoding/json"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// addRuneSentinel is a fake span URL marking the "+ attach a rune" line, so a
// mouse click there opens the rune picker (see handleFocusMouse).
const addRuneSentinel = "\x00add-rune"

// ldFlagURL is the LaunchDarkly dashboard targeting page for a flag.
func ldFlagURL(project, env, key string) string {
	if project == "" {
		project = "default"
	}
	if env == "" {
		env = "production"
	}
	return fmt.Sprintf("https://app.launchdarkly.com/projects/%s/flags/%s/targeting?env=%s&selected-env=%s", project, key, env, env)
}

// runeSearchMsg carries a rune-picker search's results back into Update.
type runeSearchMsg struct {
	query string
	items []pickerItem
}

// runesMsg carries freshly-fetched rune statuses (immediate refresh after a
// rune is attached/watched).
type runesMsg struct{ runes []RuneStatus }

// runeSearchCmd searches LaunchDarkly flags matching query, off the UI thread.
func runeSearchCmd(project, query string) tea.Cmd {
	return func() tea.Msg {
		if query == "" {
			return runeSearchMsg{query: query}
		}
		out, err := runCmd("ldcli", "flags", "list", "--project", project, "--filter", "query:"+query, "--summary", "1")
		if err != nil {
			return runeSearchMsg{query: query}
		}
		var resp ldFlagsResponse
		if json.Unmarshal(out, &resp) != nil {
			return runeSearchMsg{query: query}
		}
		var items []pickerItem
		for i, f := range resp.Items {
			if i >= 25 {
				break
			}
			label := f.Key
			if f.Name != "" && f.Name != f.Key {
				label = f.Key + "  —  " + f.Name
			}
			items = append(items, pickerItem{ID: f.Key, Label: label})
		}
		return runeSearchMsg{query: query, items: items}
	}
}

// refreshRunesCmd fetches the given flags' status immediately (after attach).
func refreshRunesCmd(project, env string, keys []string) tea.Cmd {
	return func() tea.Msg {
		var res runesMsg
		for _, k := range keys {
			if st, ok := fetchRuneStatus(project, env, k); ok {
				res.runes = append(res.runes, st)
			}
		}
		return res
	}
}

// openRunePicker opens the flag-search picker. questID attaches the chosen
// rune to that quest; "" watches it globally in the Tavern's Runes list.
func (m *Model) openRunePicker(questID string) {
	if m.modal != nil && m.modal.Kind == ModalQuestDetail {
		m.commitBodyLine()
	}
	m.clearFocusLink()
	m.pushModal(&Modal{Kind: ModalRunePicker, TargetQuestID: questID})
}

// attachRune links flag key to a quest (questID != "") or the Tavern's global
// watched list (questID == ""), deduped, and persists.
func (m *Model) attachRune(questID, key string) {
	if key == "" {
		return
	}
	if questID == "" {
		if indexOfStr(m.store.Runes, key) < 0 {
			m.store.Runes = append(m.store.Runes, key)
		}
		m.save()
		return
	}
	if q := m.findQuest(questID); q != nil {
		if indexOfStr(q.Runes, key) < 0 {
			q.Runes = append(q.Runes, key)
			q.UpdatedAt = time.Now()
		}
		m.save()
	}
}

// LaunchDarkly "runes" — feature flags you're watching. A rune's live rollout
// state is read via `ldcli` (see the launchdarkly reference): `on` + no rules
// serves the fallthrough variation ("on"); `on` + rules is a targeted/partial
// rollout ("partial"); `on: false` serves the off variation ("off"). Runes
// attach to a quest (Quest.Runes) and/or sit in the Tavern's watched list
// (Store.Runes); both are polled on the sync loop.

// RuneStatus is a flag's resolved state in production and test. The primary
// State/Served are production (drive the glyph); StateTest/ServedTest are the
// test environment, shown alongside.
type RuneStatus struct {
	Key        string // flag key, e.g. "scanneros_scan_stream_sync"
	Name       string // human name
	State      string // production: "on" | "partial" | "off"
	Served     string // production served value ("true"/"false"/…) or rule summary
	StateTest  string // test environment state
	ServedTest string // test environment served value / rule summary
}

// ldFlag is one flag from `ldcli flags list --summary 0`.
type ldFlag struct {
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
}

// ldFlagsResponse is the slice of flags ldcli returns.
type ldFlagsResponse struct {
	Items []ldFlag `json:"items"`
}

// fetchRuneStatus looks up one flag's state in the production and test
// environments. ldcli only returns one environment's targeting per call (the
// --env is required to populate it), so this makes one call per environment.
// ok is false when the production lookup fails or the flag key isn't found.
func fetchRuneStatus(project, prodEnv, key string) (RuneStatus, bool) {
	state, served, name, ok := fetchEnv(project, prodEnv, key)
	if !ok {
		return RuneStatus{}, false
	}
	st := RuneStatus{Key: key, Name: name, State: state, Served: served}
	if s, sv, _, tok := fetchEnv(project, "test", key); tok {
		st.StateTest, st.ServedTest = s, sv
	}
	return st, true
}

// fetchEnv resolves one flag's served state in a single environment via ldcli.
func fetchEnv(project, env, key string) (state, served, name string, ok bool) {
	out, err := runCmd("ldcli", "flags", "list", "--project", project,
		"--filter", "query:"+key, "--env", env, "--summary", "0")
	if err != nil {
		return "", "", "", false
	}
	var resp ldFlagsResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", "", false
	}
	for _, f := range resp.Items {
		if f.Key != key {
			continue // substring filter — take the exact-key item
		}
		if s, sv, ok := envState(f, env); ok {
			return s, sv, f.Name, true
		}
		return "", "", "", false
	}
	return "", "", "", false
}

// envState resolves one flag's served state in a single environment: off (the
// off variation), partial (targeting rules present), or on (the fallthrough).
func envState(f ldFlag, env string) (state, served string, ok bool) {
	e, present := f.Environments[env]
	if !present {
		return "", "", false
	}
	val := func(idx *int) string {
		if idx == nil || *idx < 0 || *idx >= len(f.Variations) {
			return ""
		}
		return fmt.Sprintf("%v", f.Variations[*idx].Value)
	}
	switch {
	case !e.On:
		return "off", val(e.OffVariation), true
	case len(e.Rules) > 0:
		return "partial", fmt.Sprintf("%d rule(s)", len(e.Rules)), true
	default:
		return "on", val(e.Fallthrough.Variation), true
	}
}

// runeRowContent is the "glyph key  state" text shown for one watched rune in
// the Tavern's Runes section (spliced in as the row's titleView, since the
// live rollout state lives here in the app, not in the ui package).
func (m *Model) runeRowContent(key string) string {
	return m.runeGlyph(key) + " " + key + "  " + ui.StyleMuted.Render(m.runeWord(key))
}

// unwatchRune detaches flag key from every quest (the Tavern Runes section is
// a mirror of quests, so "stop watching" means detach).
func (m *Model) unwatchRune(key string) {
	m.detachRuneFromQuest("", key)
}

// detachRuneFromQuest removes flag key from a quest's runes (all quests when
// questID is "") and persists — how a rune is dropped from the Tavern's Runes
// section (which mirrors quests).
func (m *Model) detachRuneFromQuest(questID, key string) {
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		if questID != "" && q.ID != questID {
			continue
		}
		out := q.Runes[:0]
		for _, k := range q.Runes {
			if k != key {
				out = append(out, k)
			}
		}
		q.Runes = out
	}
	m.save()
}

// collectRuneKeys is every distinct flag key to sync — the runes attached to
// un-vaulted quests. Vaulted quests go silent (their flags aren't fetched).
func (m *Model) collectRuneKeys() []string {
	seen := map[string]bool{}
	var keys []string
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		if m.isVaulted(q) {
			continue
		}
		for _, k := range q.Runes {
			if k != "" && !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
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

// runeWord is the status shown beside the icon: production state, then the
// test-environment state (so you see both at a glance).
func (m *Model) runeWord(key string) string {
	st, ok := m.runeStatus[key]
	if !ok {
		return "fetching…"
	}
	word := envWord(st.State, st.Served)
	if st.StateTest != "" {
		word += " " + ui.StyleMuted.Render("· "+envWord(st.StateTest, st.ServedTest))
	}
	return word
}

// envWord is the short status word for one environment.
func envWord(state, served string) string {
	switch state {
	case "on":
		return "on"
	case "partial":
		return served // "N rule(s)"
	case "off":
		return "off"
	default:
		return "fetching…"
	}
}
