package app

import (
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
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
	m.pastePrompt = &pastePrompt{
		codes:    codes,
		origText: map[int]string{mod.BodyCursor: value},
		line:     mod.BodyCursor,
	}
	return m.syncNow(codes)
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

	var all []string
	origText := map[int]string{}
	for i := start; i <= end; i++ {
		text := (*body)[i].Text
		if i == mod.BodyCursor {
			text = mod.BodyEditor.Value()
		}
		stripped, codes := m.captureAndStrip(q, text)
		if len(codes) == 0 {
			continue
		}
		origText[i] = text
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
	m.pastePrompt = &pastePrompt{codes: all, origText: origText, line: mod.BodyCursor}
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

	for _, code := range model.DetectJiras(text) {
		before := len(q.JiraCodes)
		appendJiraCode(q, code)
		if len(q.JiraCodes) > before {
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

// referencePendingLinks resolves an active pastePrompt as "reference": it
// un-tracks the just-captured codes and restores each affected body line to the
// text it had before the URL was stripped, so the raw URL stays inline (a
// terminal-clickable reference) instead of being tracked. The editor is
// re-homed onto the primary affected line.
func (m *Model) referencePendingLinks(q *model.Quest) {
	p := m.pastePrompt
	m.pastePrompt = nil
	if p == nil {
		return
	}
	for _, code := range p.codes {
		untrackCode(q, code)
	}
	body := m.currentBody()
	if body != nil {
		for i, text := range p.origText {
			if i >= 0 && i < len(*body) {
				(*body)[i].Text = text
			}
		}
		if p.line >= 0 && p.line < len(*body) {
			ed := m.newBodyEditor((*body)[p.line].Text)
			ed.CursorEnd()
			m.modal.BodyEditor = ed
			m.modal.BodyCursor = p.line
		}
	}
	m.touchBodyOwner()
}

// untrackCode removes a single captured code from q — a "#"-prefixed code drops
// from PRs, anything else from JiraCodes.
func untrackCode(q *model.Quest, code string) {
	if strings.HasPrefix(code, "#") {
		out := q.PRs[:0]
		for _, pr := range q.PRs {
			if pr.Code != code {
				out = append(out, pr)
			}
		}
		q.PRs = out
		return
	}
	out := q.JiraCodes[:0]
	for _, c := range q.JiraCodes {
		if c != code {
			out = append(out, c)
		}
	}
	q.JiraCodes = out
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
