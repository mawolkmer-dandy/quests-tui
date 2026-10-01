package app

import (
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// appendJiraCode adds code to q.JiraCodes unless it's already linked.
func appendJiraCode(q *model.Quest, code string) {
	for _, c := range q.JiraCodes {
		if c == code {
			return
		}
	}
	q.JiraCodes = append(q.JiraCodes, code)
}

// appendPRLink adds ref to q.PRs unless a PR with the same code is already
// linked. Dedup is by code alone — the same PR number never appears under two
// repos in practice, and matching on repo too would double-link on a URL
// rewrite.
func appendPRLink(q *model.Quest, ref model.PRRef) {
	for _, pr := range q.PRs {
		if pr.Code == ref.Code {
			return
		}
	}
	q.PRs = append(q.PRs, model.PRLink{Code: ref.Code, Repo: ref.Repo})
}

// onFocusLink reports whether the expanded quest view's link cursor is
// currently active (sitting on a Jira/PR line above the body).
func (m *Model) onFocusLink() bool {
	return m.focusLinkIdx != noSelection
}

// bodyCaretActive reports whether the editable body owns the caret in a focus
// view — i.e. no other pane is focused (not the title editor, not a Sigils
// link). The body's active-line cursor mark and the recorded screen caret must
// render only when this is true; otherwise two panes would each draw a caret.
func (m *Model) bodyCaretActive() bool {
	if m.titleEditor != nil || m.onFocusLink() {
		return false
	}
	return true
}

// focusLinkAtLine returns the index of the focus link on the given content row
// (a sigil occupies its own row), or noSelection when the row has no link.
func (m *Model) focusLinkAtLine(line int) int {
	for i, l := range m.focusLinks {
		if l.line == line {
			return i
		}
	}
	return noSelection
}

// sigilStatusLine is the action hint for the currently-focused sigil, shown on
// a fixed status line below the Sigils box (never inline — an inline hint made
// long rows wrap and shifted the layout). Empty when nothing is focused.
func (m *Model) sigilStatusLine(q *model.Quest) string {
	if !m.onFocusLink() || m.focusLinkIdx < 0 || m.focusLinkIdx >= len(m.focusLinks) {
		// Not on a sigil (editing the body): show the body-safe quest actions.
		return m.questActionHint()
	}
	if m.focusLinkConfirmID != "" {
		return ui.StyleImportant.Render("remove this link? y/n")
	}
	del := strings.ToLower(Keys.Delete.Help().Key) // "ctrl+x"
	switch m.focusLinks[m.focusLinkIdx].kind {
	case linkAddAgent:
		return keyHint("enter", "add")
	case linkCopySection:
		// Found-from-trails section headers also offer a resync (click it, or "r").
		if isTrailResyncSection(m.focusLinks[m.focusLinkIdx].code) {
			return joinHints(keyHint("c", "copy all"), keyHint("r", "resync"))
		}
		return keyHint("c", "copy all")
	case linkTrack:
		return joinHints(keyHint("enter", "write plans"), keyHint("c", "copy"), keyHint(del, "dismiss"))
	case linkLookout:
		return joinHints(keyHint("enter", "scry"), keyHint("c", "copy"), keyHint("r", "rename"), keyHint(del, "remove"))
	case linkForge:
		if m.plansBusyQuest == q.ID {
			return ui.StyleMuted.Render("writing…")
		}
		return keyHint("enter", "write plans")
	case linkRestore:
		return keyHint("enter", "restore")
	case linkAgent: // status-only — no link to copy
		return joinHints(keyHint("enter", "open"), keyHint(del, "remove"))
	default: // linkJira/linkPR/linkRune
		return joinHints(keyHint("enter", "open"), keyHint("c", "copy"), keyHint(del, "remove"))
	}
}

// questActionHint is the detail footer shown while editing the body: the quest-
// level actions that are safe here. Active / schedule / vault are omitted — they
// share keys the body editor needs (Ctrl+A/E = line start/end via Cmd+←/→,
// Ctrl+V = paste), so they act on the quest only from the Sigils pane.
func (m *Model) questActionHint() string {
	return joinHints(
		keyHint("ctrl+d", "done"),
		keyHint("ctrl+p", "priority"),
		keyHint("ctrl+t", "type"),
		keyHint("ctrl+o", "move"),
	)
}

// clearFocusLink drops the link cursor (and any armed removal), returning the
// caret to the body — call when leaving the links or closing the view.
func (m *Model) clearFocusLink() {
	m.focusLinkIdx = noSelection
	m.focusLinkConfirmID = ""
}

// handleFocusLinkKey handles keys while the link cursor is active. It relies on
// m.focusLinks being populated by the previous render — the indices there line
// up with focusLinkIdx. Returns handled=false only for keys the link cursor
// doesn't claim (so the caller can fall through to its normal handling; in
// practice every relevant key is claimed here).
func (m *Model) handleFocusLinkKey(msg tea.KeyPressMsg, q *model.Quest) (tea.Cmd, bool) {
	// Resolve the focused link against the last render's list. A stale index
	// (list shrank) just drops back to the body.
	if m.focusLinkIdx < 0 || m.focusLinkIdx >= len(m.focusLinks) {
		m.clearFocusLink()
		return nil, false
	}
	link := m.focusLinks[m.focusLinkIdx]

	// An armed removal consumes the next key as a y/n answer.
	if m.focusLinkConfirmID != "" {
		m.focusLinkConfirmID = ""
		if msg.String() == "y" {
			m.removeFocusLink(q, link)
		}
		return nil, true
	}

	switch {
	case msg.Code == tea.KeyEsc:
		m.commitBodyLine()
		m.closeModal()
		return nil, true
	case msg.Code == tea.KeyRight:
		// → leaves the Sigils pane (left) and returns to the body (right).
		m.clearFocusLink()
		m.seedBodyEditor(m.bodyCursor, 0)
		return nil, true
	case msg.Code == tea.KeyLeft:
		return nil, true // Sigils is the leftmost pane
	case msg.String() == "up":
		if m.focusLinkIdx > 0 {
			m.focusLinkIdx--
		} else {
			// At the top of Sigils, Up jumps to the title for renaming.
			m.beginTitleEdit()
		}
		return nil, true
	case msg.String() == "down":
		// Bound by the ACTUAL rendered stops (m.focusLinks), not the recomputed
		// focusLinkCount — the latter omits the section-header copy stops, so it
		// stopped Down early and left click-focused lines unreachable going down.
		if m.focusLinkIdx < len(m.focusLinks)-1 {
			m.focusLinkIdx++
		}
		return nil, true
	case (link.kind == linkTrack || link.kind == linkForge) && msg.String() == "i":
		// Write the Lookout's plans on demand (a cheap Claude call).
		return m.forgePlans(q), true
	case link.kind == linkLookout && msg.String() == "r":
		// Rename the dashboard inline (its name can't be scraped from the URL).
		m.beginLookoutRename(q.ID, link.code)
		return nil, true
	case link.kind == linkCopySection && isTrailResyncSection(link.code) && msg.String() == "r":
		// Resync from any found-from-trails header: refetch PR status + re-scan.
		return m.resyncTrails(q), true
	case msg.String() == "c":
		// Copy: a section header → the whole section as a list; any other item
		// → just its link.
		return m.copyFocusLink(q, link), true
	case msg.Code == tea.KeyEnter:
		switch link.kind {
		case linkAddAgent:
			return m.openAgentPicker(), true
		case linkRestore:
			return m.restoreDismissedTracks(q.ID), true
		case linkCopySection:
			return m.copyFocusLink(q, link), true
		case linkForge, linkTrack:
			// Both write the quest's Lookout plans (a track's only action beyond
			// dismiss is to contribute to the plans).
			return m.forgePlans(q), true
		case linkLookout:
			return openURL(link.url), true
		case linkAgent, linkJira, linkPR, linkRune:
			// One shared "open" — agents focus their pane, the rest open in the
			// browser — identical to the Tavern (see openConnection).
			return m.openConnection(connection{kind: link.kind, code: link.code, url: link.url}), true
		}
	case key.Matches(msg, Keys.Delete):
		if link.kind == linkAddAgent || link.kind == linkForge || link.kind == linkRestore || link.kind == linkCopySection {
			return nil, true // nothing to remove on an affordance / header line
		}
		m.focusLinkConfirmID = link.code
		return nil, true
	}
	// Any other key drops back to the body so typing isn't swallowed here.
	m.clearFocusLink()
	return nil, false
}

// removeFocusLink removes link from q and persists: a Jira/PR code drops from
// its slice, an agent worktree unpins. The link cursor is re-homed onto a
// surviving link (the "+ add agent" line always remains, so there's always at
// least one).
func (m *Model) removeFocusLink(q *model.Quest, link focusLink) {
	switch link.kind {
	case linkJira:
		out := q.JiraCodes[:0]
		for _, c := range q.JiraCodes {
			if c != link.code {
				out = append(out, c)
			}
		}
		q.JiraCodes = out
	case linkPR:
		out := q.PRs[:0]
		for _, pr := range q.PRs {
			if pr.Code != link.code {
				out = append(out, pr)
			}
		}
		q.PRs = out
	case linkAgent:
		out := q.AgentWorkspaces[:0]
		for _, id := range q.AgentWorkspaces {
			if id != link.code {
				out = append(out, id)
			}
		}
		q.AgentWorkspaces = out
	case linkRune:
		out := q.Runes[:0]
		for _, k := range q.Runes {
			if k != link.code {
				out = append(out, k)
			}
		}
		q.Runes = out
	case linkTrack:
		out := q.Tracks[:0]
		for _, t := range q.Tracks {
			if t.Event != link.code {
				out = append(out, t)
			}
		}
		q.Tracks = out
		if indexOfStr(q.DismissedTracks, link.code) < 0 {
			q.DismissedTracks = append(q.DismissedTracks, link.code) // don't re-harvest it
		}
	case linkLookout:
		out := q.Lookouts[:0]
		for _, l := range q.Lookouts {
			if l.URL != link.code {
				out = append(out, l)
			}
		}
		q.Lookouts = out
	}
	m.touchBodyOwner()

	remaining := m.focusLinkCount(q)
	if remaining == 0 {
		m.clearFocusLink()
		m.seedBodyEditor(0, 0)
		return
	}
	if m.focusLinkIdx >= remaining {
		m.focusLinkIdx = remaining - 1
	}
}

// captureCurrentBodyLink inspects the live editor value of the current body
// line for a COMPLETE Jira/PR URL. Any it finds are captured onto q (JiraCodes /
// PRs), the URL text is stripped out of the line (so the raw URL doesn't linger
// where it was pasted), the editor is reseeded with the stripped text, a
// pastePrompt is armed (so the next key can keep it inline instead), and an
// immediate sync for just the newly-captured code(s) is returned. Returns nil
// when nothing new was captured. Only meaningful for ModalQuestDetail.
func (m *Model) captureCurrentBodyLink(q *model.Quest) tea.Cmd {
	mod := m.modal
	if mod == nil || mod.Kind != ModalQuestDetail {
		return nil
	}
	body := m.currentBody()
	if body == nil || m.bodyCursor < 0 || m.bodyCursor >= len(*body) {
		return nil
	}

	value := m.bodyEditor.Value()
	stripped, codes, runes, lookouts, prs, changed := m.captureAndStrip(q, value)
	if !changed {
		return nil // no link captured or shortened — leave the line (and its spaces) alone
	}

	// Reseed the line + editor with captured URLs removed / long links shortened,
	// keeping the caret at the end of what remains.
	(*body)[m.bodyCursor].Text = stripped
	ed := m.newBodyEditor(stripped)
	ed.CursorEnd()
	m.bodyEditor = ed
	m.touchBodyOwner()
	return m.captureSync(q.ID, codes, runes, lookouts, prs)
}

// captureSync fetches the just-captured PR/Jira codes, refreshes the
// just-captured runes, and registers just-captured dashboard Lookouts,
// animating the "fetching" state meanwhile.
func (m *Model) captureSync(questID string, codes, runes, lookouts []string, prs []model.PRLink) tea.Cmd {
	var cmds []tea.Cmd
	if len(codes) > 0 {
		cmds = append(cmds, m.syncNow(codes))
	}
	if len(runes) > 0 {
		cmds = append(cmds, refreshRunesCmd(m.ldProject, m.ldEnv, runes), m.maybeStartSpinner())
	}
	// A pasted PR may be one rung of a Graphite stack — pull in its siblings.
	for _, pr := range prs {
		cmds = append(cmds, stackExpandCmd(questID, pr))
	}
	if len(codes) > 0 || len(runes) > 0 || len(lookouts) > 0 {
		// a Jira/PR/rune/dashboard link was just captured: sound + a sparkle burst
		// on its Sigils line (deferred to the next render — pendingConnBurstCode).
		switch {
		case len(codes) > 0:
			m.pendingConnBurstCode = codes[0]
		case len(runes) > 0:
			m.pendingConnBurstCode = runes[0]
		default:
			m.pendingConnBurstCode = lookouts[0]
		}
		cmds = append(cmds, m.playSound(sndAddConnection), m.pokeOverlayTick())
	}
	return tea.Batch(cmds...)
}

// captureBodyLinesRange captures links across body lines [start, end] only
// (the lines a multiline paste just produced — scanning the whole body would
// re-capture pre-existing inline references), stripping each URL out of its
// line. Returns an immediate sync for the new code(s), or nil when nothing new
// was captured.
func (m *Model) captureBodyLinesRange(q *model.Quest, start, end int) tea.Cmd {
	mod := m.modal
	if mod == nil || mod.Kind != ModalQuestDetail {
		return nil
	}
	body := m.currentBody()
	if body == nil {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if end >= len(*body) {
		end = len(*body) - 1
	}

	var allCodes, allRunes, allLookouts []string
	var allPRs []model.PRLink
	changed := false
	for i := start; i <= end; i++ {
		text := (*body)[i].Text
		if i == m.bodyCursor {
			text = m.bodyEditor.Value()
		}
		stripped, codes, runes, lookouts, prs, lineChanged := m.captureAndStrip(q, text)
		if !lineChanged {
			continue
		}
		changed = true
		(*body)[i].Text = stripped
		if i == m.bodyCursor {
			ed := m.newBodyEditor(stripped)
			ed.CursorEnd()
			m.bodyEditor = ed
		}
		allCodes = append(allCodes, codes...)
		allRunes = append(allRunes, runes...)
		allLookouts = append(allLookouts, lookouts...)
		allPRs = append(allPRs, prs...)
	}
	if !changed {
		return nil
	}
	m.touchBodyOwner()
	return m.captureSync(q.ID, allCodes, allRunes, allLookouts, allPRs)
}

// captureAndStrip captures every Jira/PR/LaunchDarkly URL in text onto q
// (JiraCodes / PRs / Runes, each deduped) and returns the text with those URLs
// REMOVED entirely — a pasted link is pulled into the quest's connections, not
// left inline. newCodes/newRunes list what was NEWLY captured this call (so a
// re-detected, already-linked reference doesn't trigger a redundant fetch).
func (m *Model) captureAndStrip(q *model.Quest, text string) (stripped string, newCodes, newRunes, newLookouts []string, newPRs []model.PRLink, changed bool) {
	stripped = model.StripLinks(text)

	jiras := model.DetectJiras(text)
	for _, code := range jiras {
		before := len(q.JiraCodes)
		appendJiraCode(q, code)
		if len(q.JiraCodes) > before {
			newCodes = append(newCodes, code)
		}
	}
	prs := model.DetectPRs(text)
	for _, ref := range prs {
		before := len(q.PRs)
		appendPRLink(q, ref)
		if len(q.PRs) > before {
			newCodes = append(newCodes, ref.Code)
			newPRs = append(newPRs, model.PRLink{Code: ref.Code, Repo: ref.Repo})
		}
	}
	// Runes (LaunchDarkly flags) are no longer captured from a pasted link —
	// they're found from a PR's body during a track-find (see find.go).
	dashboards := model.DetectDashboards(text)
	for _, url := range dashboards {
		if lookoutIndexOf(q.Lookouts, url) < 0 {
			q.Lookouts = append(q.Lookouts, model.Lookout{URL: url, Tool: inferTool(url), AddedAt: time.Now()})
			newLookouts = append(newLookouts, url)
		}
	}
	// Any remaining bare URL (not captured to a sigil) is shortened inline and
	// recorded in q.BodyLinks so it stays a compact, clickable link.
	stripped, shortened := m.shortenBodyLinks(q, stripped)
	// changed is true only when a link was captured (StripLinks removed a
	// Jira/PR/dashboard URL, new or duplicate) or a free URL was shortened —
	// NOT for StripLinks's cosmetic whitespace trimming, so an ordinary typed
	// trailing space isn't eaten by a needless reseed.
	changed = shortened || len(jiras) > 0 || len(prs) > 0 || len(dashboards) > 0
	return stripped, newCodes, newRunes, newLookouts, newPRs, changed
}

// trackedCodeURLs maps each of q's tracked codes (Jira issues and PRs) to the
// URL it opens — used to make the shortened codes left inline in the body
// clickable (see renderBodyLineWrapped).
func (m *Model) trackedCodeURLs(q *model.Quest) map[string]string {
	out := make(map[string]string, len(q.JiraCodes)+len(q.PRs)+len(q.BodyLinks))
	for _, c := range q.JiraCodes {
		out[c] = jiraURL(c, m.jiraBaseURL)
	}
	for _, pr := range q.PRs {
		out[pr.Code] = prURL(pr.Repo, pr.Code)
	}
	// Shortened inline links (non-captured URLs) are clickable too.
	for short, url := range q.BodyLinks {
		out[short] = url
	}
	return out
}

// openURL opens url in the system browser, fire-and-forget — a failed launch
// is silently ignored (there's nothing useful to surface for it in the TUI).
func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command("open", url).Start()
		return nil
	}
}

// jiraURL builds the browse URL for a Jira issue key against base (the
// configured Jira base, e.g. "https://meetdandy.atlassian.net").
func jiraURL(code, base string) string {
	return strings.TrimRight(base, "/") + "/browse/" + code
}

// prURL builds the pull-request URL from "owner/repo" and a PR number (with
// any leading "#" stripped).
func prURL(repo, num string) string {
	return "https://github.com/" + repo + "/pull/" + strings.TrimPrefix(num, "#")
}
