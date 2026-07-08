package app

import (
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// captureLinks scans a quest's body lines in order and fills in its
// integration links: the FIRST Jira URL sets JiraCode (if empty); EVERY GitHub
// PR URL is appended to PRs (deduped by code), in reading order. Once a code is
// present it's left alone, so re-running is idempotent — called from
// commitBodyLine (ModalQuestDetail only) as a safety net over the instant
// paste-time capture (see updateModal).
func (m *Model) captureLinks(q *model.Quest) {
	for _, l := range q.Body {
		if q.JiraCode == "" {
			if code, ok := model.DetectJira(l.Text); ok {
				q.JiraCode = code
			}
		}
		for _, ref := range model.DetectPRs(l.Text) {
			appendPRLink(q, ref)
		}
	}
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
func (m *Model) handleFocusLinkKey(msg tea.KeyMsg, q *model.Quest) (tea.Cmd, bool) {
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
	case msg.Type == tea.KeyEsc:
		m.commitBodyLine()
		m.closeModal()
		return nil, true
	case msg.String() == "up":
		if m.focusLinkIdx > 0 {
			m.focusLinkIdx--
		}
		return nil, true
	case msg.String() == "down":
		// Off the bottom link, return to the top body line.
		if m.focusLinkIdx < m.focusLinkCount(q)-1 {
			m.focusLinkIdx++
			return nil, true
		}
		m.clearFocusLink()
		m.seedBodyEditor(0, 0)
		return nil, true
	case msg.Type == tea.KeyEnter:
		return openURL(link.url), true
	case key.Matches(msg, Keys.Delete):
		m.focusLinkConfirmID = link.code
		return nil, true
	}
	// Any other key drops back to the body so typing isn't swallowed here.
	m.clearFocusLink()
	return nil, false
}

// removeFocusLink removes link from q and persists: a PR drops from q.PRs, the
// Jira clears q.JiraCode. The link cursor is re-homed onto a surviving link, or
// back to the body when none remain.
func (m *Model) removeFocusLink(q *model.Quest, link focusLink) {
	switch link.kind {
	case linkJira:
		q.JiraCode = ""
	case linkPR:
		out := q.PRs[:0]
		for _, pr := range q.PRs {
			if pr.Code != link.code {
				out = append(out, pr)
			}
		}
		q.PRs = out
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
// line for a COMPLETE Jira/PR URL. Any it finds are captured onto q (JiraCode /
// PRs), the URL text is stripped out of the line (so the raw URL doesn't linger
// where it was pasted), the editor is reseeded with the stripped text, and an
// immediate sync for just the newly-captured code(s) is returned. Returns nil
// when nothing new was captured. Only meaningful for ModalQuestDetail.
func (m *Model) captureCurrentBodyLink(q *model.Quest) tea.Cmd {
	mod := m.modal
	if mod == nil || mod.Kind != ModalQuestDetail {
		return nil
	}
	body := m.currentBody()
	if body == nil || mod.BodyCursor < 0 || mod.BodyCursor >= len(*body) {
		return nil
	}

	value := mod.BodyEditor.Value()
	stripped, codes := m.captureAndStrip(q, value)
	if len(codes) == 0 {
		return nil
	}

	// Reseed the line + editor with the stripped text, keeping the caret at the
	// end of what remains (the URL was almost always the tail being typed).
	(*body)[mod.BodyCursor].Text = stripped
	ed := m.newBodyEditor(stripped)
	ed.CursorEnd()
	mod.BodyEditor = ed
	m.touchBodyOwner()
	return m.syncNow(codes)
}

// captureAllBodyLinks captures links across EVERY body line (used after a
// multiline paste, whose lines are already committed to the body), stripping
// each URL out of its line. Returns an immediate sync for the new code(s), or
// nil when nothing new was captured.
func (m *Model) captureAllBodyLinks(q *model.Quest) tea.Cmd {
	mod := m.modal
	if mod == nil || mod.Kind != ModalQuestDetail {
		return nil
	}
	body := m.currentBody()
	if body == nil {
		return nil
	}

	var all []string
	for i := range *body {
		text := (*body)[i].Text
		if i == mod.BodyCursor {
			text = mod.BodyEditor.Value()
		}
		stripped, codes := m.captureAndStrip(q, text)
		if len(codes) == 0 {
			continue
		}
		(*body)[i].Text = stripped
		if i == mod.BodyCursor {
			ed := m.newBodyEditor(stripped)
			ed.CursorEnd()
			mod.BodyEditor = ed
		}
		all = append(all, codes...)
	}
	if len(all) == 0 {
		return nil
	}
	m.touchBodyOwner()
	return m.syncNow(all)
}

// captureAndStrip captures every Jira/PR URL in text onto q (first Jira only;
// every PR, deduped), returns the text with those URLs removed and tidied, and
// the list of codes that were NEWLY captured this call (so a re-detected,
// already-linked code doesn't trigger a redundant fetch).
func (m *Model) captureAndStrip(q *model.Quest, text string) (stripped string, newCodes []string) {
	if _, refs := model.ShortenLinks(text); len(refs) == 0 {
		return text, nil
	}

	if code, ok := model.DetectJira(text); ok {
		if q.JiraCode == "" {
			q.JiraCode = code
			newCodes = append(newCodes, code)
		}
	}
	for _, ref := range model.DetectPRs(text) {
		before := len(q.PRs)
		appendPRLink(q, ref)
		if len(q.PRs) > before {
			newCodes = append(newCodes, ref.Code)
		}
	}

	// Remove the raw URLs from the line, tidying the whitespace left behind.
	return model.StripLinks(text), newCodes
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
