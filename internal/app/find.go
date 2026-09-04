package app

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// Finding scans a quest's linked Trails (PRs) for Tracks (tracking events),
// fully programmatically (no AI): events come from where they're defined — the
// Zod schema entries `'Event Name': z.object({ prop: … })` in the PR diff → the
// event name + its property "marks" — plus LaunchDarkly flags and Jira issues
// from each PR body. Only items not already on the quest are added, honoring
// Quest.DismissedTracks. Triggered explicitly (Keys.Find / the affordance) and
// automatically on sync when a PR's head SHA changes.

// findTimeout bounds each `gh` call during a find — longer than the sync
// timeout since a big `gh pr diff` can be slow.
const findTimeout = 30 * time.Second

// findMsg carries what a find turned up back into Update to be applied.
type findMsg struct {
	questID string
	runes   []string
	runeSrc map[string]string // rune key → source PR "#code" (for aging)
	jiras   []string
	tracks  []model.Track
}

// eventStartRE matches the start of a Zod event definition — the quoted event
// name followed by `: z`, whether the `.object({` sits on the same line
// (`'Event': z.object({`) or prettier wrapped it onto the next
// (`'Event': z` ⏎ `.object({`). The name is validated by looksLikeEventName.
var eventStartRE = regexp.MustCompile(`^['"]([^'"]+)['"]\s*:\s*z(?:\.|\s*$)`)

// eventPropRE matches a Zod property line inside an event object, e.g.
// `source: z.string(),` — capturing the property key.
var eventPropRE = regexp.MustCompile(`^(\w+)\s*:\s*z\.`)

// flagKeyRE matches a top-level LaunchDarkly flag declaration in
// launch-darkly.types.ts — a snake_case key mapped to a Zod type, e.g.
// `scanneros_impressions_smokescreen_enabled: z.boolean(),`.
var flagKeyRE = regexp.MustCompile(`^([a-z][a-z0-9_]*)\s*:\s*z\.`)

// findTracksInTrails fetches each linked PR's body + diff and returns a command
// that reports what to add. Off the UI thread; applied in Update (see findMsg).
func (m *Model) findTracksInTrails(questID string) tea.Cmd {
	q := m.findQuest(questID)
	if q == nil {
		return nil
	}
	if len(q.PRs) == 0 {
		return m.showClipboardToastText("no trails to search")
	}
	m.findingQuestID = questID
	m.invalidateRender()
	return findCmd(questID, q.PRs)
}

// findCmd does the off-thread `gh` fetches for one quest's PRs. Shared by the
// explicit trigger and the auto-on-sync path.
func findCmd(questID string, prs []model.PRLink) tea.Cmd {
	prsCopy := append([]model.PRLink(nil), prs...)
	return func() tea.Msg {
		res := findMsg{questID: questID, runeSrc: map[string]string{}}
		for _, pr := range prsCopy {
			num := strings.TrimPrefix(pr.Code, "#")
			if body, err := runCmdTimeout(findTimeout, "gh", "pr", "view", num, "--repo", pr.Repo, "--json", "body", "--jq", ".body"); err == nil {
				text := string(body)
				for _, k := range model.DetectLDFlags(text) {
					res.runes = append(res.runes, k)
					if _, ok := res.runeSrc[k]; !ok {
						res.runeSrc[k] = pr.Code
					}
				}
				res.jiras = append(res.jiras, model.DetectJiras(text)...)
			}
			if diff, err := runCmdTimeout(findTimeout, "gh", "pr", "diff", num, "--repo", pr.Repo); err == nil {
				d := string(diff)
				found := parseEventsFromDiff(d)
				for i := range found {
					found[i].SourcePR = pr.Code // provenance → track's production color
				}
				res.tracks = append(res.tracks, found...)
				// Runes: flag keys declared in launch-darkly.types.ts (ldcli
				// resolves each key's live status on the sync loop).
				for _, k := range parseFlagsFromDiff(d) {
					res.runes = append(res.runes, k)
					if _, ok := res.runeSrc[k]; !ok {
						res.runeSrc[k] = pr.Code
					}
				}
			}
		}
		return res
	}
}

// autoFindCmd finds tracks for any un-vaulted quest whose linked PR head changed
// since it was last scanned — SHA-gated, so an unchanged PR costs nothing. Run
// after each sync pass applies fresh PR statuses.
func (m *Model) autoFindCmd() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		if m.isVaulted(q) || len(q.PRs) == 0 {
			continue
		}
		changed := false
		for _, pr := range q.PRs {
			st, ok := m.prStatus[pr.Code]
			if !ok || st.HeadSHA == "" {
				continue
			}
			key := pr.Repo + pr.Code
			if m.lastFoundSHA[key] != st.HeadSHA {
				m.lastFoundSHA[key] = st.HeadSHA
				changed = true
			}
		}
		if changed {
			cmds = append(cmds, findCmd(q.ID, q.PRs))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// detailOpenFor reports whether the quest-detail page for questID is the open
// modal — used to give add-connection feedback (sound + sparkle + toast) only
// when the user is actually looking at that quest, not on a background pull.
func (m *Model) detailOpenFor(questID string) bool {
	return m.modal != nil && m.modal.Kind == ModalQuestDetail && m.modal.QuestID == questID
}

// applyFind captures a find's results onto the quest — deduped against what's
// already linked and skipping dismissed tracks — persists, refreshes the
// newly-linked flags/issues, and (only when that quest's detail page is open)
// gives the same sound + sparkle + toast feedback as pasting a link.
func (m *Model) applyFind(msg findMsg) tea.Cmd {
	if m.findingQuestID == msg.questID {
		m.findingQuestID = ""
		m.invalidateRender()
	}
	q := m.findQuest(msg.questID)
	if q == nil {
		return nil
	}

	var newRunes, newJiras, newTracks []string

	for _, k := range dedupeStrings(msg.runes) {
		if indexOfStr(q.Runes, k) < 0 {
			q.Runes = append(q.Runes, k)
			newRunes = append(newRunes, k)
		}
		if src := msg.runeSrc[k]; src != "" {
			if q.RuneSources == nil {
				q.RuneSources = map[string]string{}
			}
			if q.RuneSources[k] == "" {
				q.RuneSources[k] = src // record provenance for aging
			}
		}
	}
	for _, c := range dedupeStrings(msg.jiras) {
		before := len(q.JiraCodes)
		appendJiraCode(q, c)
		if len(q.JiraCodes) > before {
			newJiras = append(newJiras, c)
		}
	}
	for _, t := range msg.tracks {
		if indexOfStr(q.DismissedTracks, t.Event) >= 0 {
			continue // the user dismissed this one; don't resurrect it
		}
		if existing, idx := findTrack(q, t.Event); idx < 0 {
			q.Tracks = append(q.Tracks, t)
			newTracks = append(newTracks, t.Event)
		} else if existing.SourcePR == "" && t.SourcePR != "" {
			existing.SourcePR = t.SourcePR // backfill provenance on older tracks
		}
	}

	if len(newRunes)+len(newJiras)+len(newTracks) == 0 {
		return nil // nothing new — stay quiet (auto-find runs often)
	}
	m.save()
	m.invalidateRender() // refresh so new emblems show even when feedback is silent

	var cmds []tea.Cmd
	if len(newRunes) > 0 {
		cmds = append(cmds, refreshRunesCmd(m.ldProject, m.ldEnv, newRunes), m.maybeStartSpinner())
	}
	if len(newJiras) > 0 {
		cmds = append(cmds, m.syncNow(newJiras))
	}
	// Feedback (same as pasting a link: sound + sparkle burst + toast) only when
	// the quest's detail page is open — a background pull stays silent.
	if m.detailOpenFor(msg.questID) {
		switch {
		case len(newTracks) > 0:
			m.pendingConnBurstCode = newTracks[0]
		case len(newRunes) > 0:
			m.pendingConnBurstCode = newRunes[0]
		default:
			m.pendingConnBurstCode = newJiras[0]
		}
		cmds = append(cmds,
			m.playSound(sndAddConnection),
			m.showClipboardToastText(fmt.Sprintf("found %d track · %d rune · %d scroll", len(newTracks), len(newRunes), len(newJiras))),
			m.pokeOverlayTick(),
		)
	}
	return tea.Batch(cmds...)
}

// graphiteStackRE pulls each PR number from a Graphite "Current stack" comment,
// whose entries are lines like `* **#48708**`. See stackExpandCmd.
var graphiteStackRE = regexp.MustCompile(`(?m)^\*+\s*\*\*#(\d+)\*\*`)

// stackMsg carries the PR numbers found in a pasted PR's Graphite stack.
type stackMsg struct {
	questID string
	repo    string
	codes   []string
}

// stackExpandCmd looks up a pasted PR's Graphite stack comment and returns the
// other PRs in the same stack, so linking one rung links the whole stack.
// Graphite posts the same "Current stack" comment (marker: a graphite.dev link)
// on every PR in the stack, listing all members — and it survives merge, unlike
// branch-walking. No-op (nil) when there's no such comment.
func stackExpandCmd(questID string, pr model.PRLink) tea.Cmd {
	num := strings.TrimPrefix(pr.Code, "#")
	repo := pr.Repo
	return func() tea.Msg {
		out, err := runCmdTimeout(findTimeout, "gh", "api",
			fmt.Sprintf("repos/%s/issues/%s/comments", repo, num),
			"--jq", `.[] | select(.body | contains("graphite.dev")) | .body`)
		if err != nil {
			return nil
		}
		var codes []string
		seen := map[string]bool{}
		for _, mm := range graphiteStackRE.FindAllStringSubmatch(string(out), -1) {
			c := "#" + mm[1]
			if !seen[c] {
				seen[c] = true
				codes = append(codes, c)
			}
		}
		if len(codes) == 0 {
			return nil
		}
		return stackMsg{questID: questID, repo: repo, codes: codes}
	}
}

// autoStackExpandCmd re-reads each active quest's Graphite stack on every sync
// pass, so a PR stacked onto the chain AFTER the quest was first linked gets
// pulled in too — captureSync only expands the stack as it stood when a rung
// was pasted. Graphite posts the same "Current stack" comment on every member,
// so one still-open member's comment lists the whole current stack; we query a
// single representative per quest (see stackExpandSeed). Vaulted quests and
// fully-settled stacks (every linked PR merged/closed) are skipped — a finished
// stack can't grow, so it costs no `gh` calls. The stackMsg handler dedupes, so
// an unchanged stack adds nothing and stays silent on a background pass.
func (m *Model) autoStackExpandCmd() tea.Cmd {
	if !m.integrationsEnabled {
		return nil
	}
	var cmds []tea.Cmd
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		if m.isVaulted(q) || len(q.PRs) == 0 {
			continue
		}
		seed, ok := m.stackExpandSeed(q)
		if !ok {
			continue // every linked PR is settled — the stack is stable
		}
		cmds = append(cmds, stackExpandCmd(q.ID, seed))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// stackExpandSeed picks the PR whose Graphite stack comment a quest's current
// stack should be read from: the first linked PR that isn't known merged/closed
// (its comment is the one Graphite still updates), or — when none has a cached
// status yet (e.g. just linked) — the first such unknown PR. Returns ok=false
// only when every linked PR is confirmed settled, so a finished stack triggers
// no `gh` call.
func (m *Model) stackExpandSeed(q *model.Quest) (model.PRLink, bool) {
	var fallback model.PRLink
	haveFallback := false
	for _, pr := range q.PRs {
		st, ok := m.prStatus[pr.Code]
		if !ok {
			if !haveFallback {
				fallback, haveFallback = pr, true // status not fetched yet — usable seed
			}
			continue
		}
		if st.Status != "merged" && st.Status != "closed" {
			return pr, true // an open member — freshest stack comment
		}
	}
	return fallback, haveFallback
}

// restoreDismissedTracks clears a quest's dismissed set and re-finds, so any
// track dismissed by mistake comes back.
func (m *Model) restoreDismissedTracks(questID string) tea.Cmd {
	q := m.findQuest(questID)
	if q == nil || len(q.DismissedTracks) == 0 {
		return nil
	}
	q.DismissedTracks = nil
	q.UpdatedAt = time.Now()
	m.save()
	return m.findTracksInTrails(questID)
}

// parseEventsFromDiff scans a unified `gh pr diff` for ADDED Zod event
// definitions and returns a Track per event, with its property keys as marks.
// It tracks object brace depth so nested objects don't leak their keys up as
// top-level marks, and only reads added ("+") lines so a track reflects what
// the PR actually introduced.
func parseEventsFromDiff(diff string) []model.Track {
	var tracks []model.Track
	var cur *model.Track
	depth := 0
	opened := false // has the event's z.object({ actually opened yet

	closeCur := func() {
		if cur != nil {
			tracks = append(tracks, *cur)
		}
		cur, depth, opened = nil, 0, false
	}
	// apply folds one added line into the current event: record a top-level
	// property key (only at object depth 1), then track brace nesting so nested
	// objects don't leak their keys and the definition closes at the matching
	// brace. `opened` guards the gap between the name line and the `.object({`.
	apply := func(content string) {
		if depth == 1 {
			if pm := eventPropRE.FindStringSubmatch(content); pm != nil {
				cur.Marks = append(cur.Marks, pm[1])
			}
		}
		depth += braceDelta(content)
		if depth >= 1 {
			opened = true
		}
		if opened && depth <= 0 {
			closeCur()
		}
	}

	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++") || !strings.HasPrefix(line, "+") {
			if cur != nil {
				closeCur() // left the added block before the object closed
			}
			continue
		}
		content := strings.TrimSpace(line[1:])

		if cur == nil {
			if mm := eventStartRE.FindStringSubmatch(content); mm != nil && looksLikeEventName(mm[1]) {
				cur = &model.Track{Event: mm[1]}
				depth, opened = 0, false
				apply(content) // handles the same-line `z.object({` case too
			}
			continue
		}
		apply(content)
	}
	closeCur()
	return tracks
}

// parseFlagsFromDiff scans a `gh pr diff` for LaunchDarkly flag keys DECLARED
// in a launch-darkly.types.ts file — the top-level keys of the flags Zod object
// (nested keys, e.g. a config object's fields, are skipped via brace depth). A
// non-added line resets depth, since a flag declaration is a contiguous added
// block. Returns the flag keys, order-preserving.
func parseFlagsFromDiff(diff string) []string {
	var keys []string
	inFile := false
	depth := 0
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++ ") {
			inFile = strings.Contains(line, "launch-darkly.types")
			depth = 0
			continue
		}
		if !inFile {
			continue
		}
		if !strings.HasPrefix(line, "+") {
			depth = 0 // left the added block (context / hunk header / removed line)
			continue
		}
		content := strings.TrimSpace(line[1:])
		if depth == 0 {
			if mm := flagKeyRE.FindStringSubmatch(content); mm != nil {
				keys = append(keys, mm[1])
			}
		}
		depth += braceDelta(content)
		if depth < 0 {
			depth = 0
		}
	}
	return keys
}

// braceDelta is the net change in object nesting a line contributes.
func braceDelta(s string) int {
	return strings.Count(s, "{") - strings.Count(s, "}")
}

// looksLikeEventName filters captured schema keys to strings that read like a
// Dandy tracking-event name ("Category - Action"): a multi-word phrase starting
// with a capital, not a camelCase identifier or a template/expression fragment.
func looksLikeEventName(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 3 || len(s) > 120 {
		return false
	}
	if !unicode.IsUpper([]rune(s)[0]) {
		return false
	}
	if !strings.Contains(s, " ") {
		return false
	}
	if strings.ContainsAny(s, "{}$`") {
		return false
	}
	return true
}

// dedupeStrings returns xs with duplicates and empties removed, order preserved.
func dedupeStrings(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}
