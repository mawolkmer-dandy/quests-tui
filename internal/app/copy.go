package app

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// Section keys used by copyable sigil headers (focusLink.code for a
// linkCopySection) and by copySection.
const (
	secNPCs     = "npcs"
	secScrolls  = "scrolls"
	secTrails   = "trails"
	secRunes    = "runes"
	secTracks   = "tracks"
	secLookouts = "lookouts"
)

// copyToClipboard writes text and flashes the "copied" toast; a no-op (no
// toast) when text is empty so an empty section doesn't pretend it copied.
func (m *Model) copyToClipboard(text, toast string) tea.Cmd {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_ = clipboard.WriteAll(text)
	return m.showClipboardToastText(toast)
}

// linkDoubleClickWindow is how close two clicks on the same link must be for
// the second to count as "open" rather than a fresh "copy".
const linkDoubleClickWindow = 400 * time.Millisecond

// clickLink implements the shared link-click gesture: the first click copies
// the URL; a second click on the same URL within the window opens it in the
// browser (and disarms, so a third click copies again).
func (m *Model) clickLink(url string) tea.Cmd {
	now := time.Now()
	if url == m.lastLinkClickURL && now.Sub(m.lastLinkClickAt) < linkDoubleClickWindow {
		m.lastLinkClickURL = ""
		return openURL(url)
	}
	m.lastLinkClickURL = url
	m.lastLinkClickAt = now
	return m.copyToClipboard(url, "link copied")
}

// copyTrack copies one track as its event + marks snippet (tracks have no URL,
// so this is their "copy" everywhere — detail Sigils, section page, Tavern).
func (m *Model) copyTrack(questID, event string) tea.Cmd {
	q := m.findQuest(questID)
	if q == nil {
		return nil
	}
	return m.copyToClipboard(trackShareText(m.findTrack(q, event)), "event copied")
}

// copyFocusLink copies the focused sigil: a section header copies the whole
// section as a shareable list; any other item copies just its link.
func (m *Model) copyFocusLink(q *model.Quest, link focusLink) tea.Cmd {
	if link.kind == linkCopySection {
		return m.copyToClipboard(m.copySection(q, link.code), "section copied")
	}
	text, label := m.copyItem(q, link)
	return m.copyToClipboard(text, label)
}

// copyItem is the clipboard text for a single sigil — the plain link for
// anything with a URL, or the event + its marks for a track.
func (m *Model) copyItem(q *model.Quest, link focusLink) (text, toast string) {
	switch link.kind {
	case linkTrack:
		return trackShareText(m.findTrack(q, link.code)), "event copied"
	case linkAgent:
		return m.agentLabel(link.code), "agent copied"
	default:
		return link.url, "link copied"
	}
}

// copySection builds the shareable list for one section (see the design in the
// PR description): Trails carry title + churn, everything else is link-only,
// Tracks list each event with its marks object.
func (m *Model) copySection(q *model.Quest, section string) string {
	var lines []string
	switch section {
	case secTrails:
		dominant := dominantRepo(q.PRs)
		for _, node := range m.prStack(q.PRs) {
			lines = append(lines, m.prShareLine(node.link, dominant))
		}
	case secScrolls:
		for _, code := range q.JiraCodes {
			lines = append(lines, mdLink(code+jiraSummarySuffix(m.jiraStatus[code]), jiraURL(code, m.jiraBaseURL)))
		}
	case secRunes:
		for _, key := range q.Runes {
			lines = append(lines, mdLink(key, ldFlagURL(m.ldProject, m.ldEnv, key)))
		}
	case secTracks:
		for _, t := range q.Tracks {
			lines = append(lines, trackShareText(t))
		}
	case secLookouts:
		for _, l := range q.Lookouts {
			lines = append(lines, l.URL)
		}
	case secNPCs:
		for _, id := range q.AgentWorkspaces {
			lines = append(lines, m.agentLabel(id))
		}
	}
	return strings.Join(lines, "\n")
}

// prShareLine is one line of a copied Trails list, in Graphite's pasteable
// markdown-stack format: a "[#code title](url)" link (the ref is owner/repo#code
// when it's not the quest's dominant repo) followed by the "+adds/-dels" churn
// in inline code — so it pastes as clickable text into a PR/doc/Linear.
func (m *Model) prShareLine(pr model.PRLink, dominantRepo string) string {
	ref := pr.Code
	if pr.Repo != "" && pr.Repo != dominantRepo {
		ref = pr.Repo + pr.Code // e.g. "orthly/unified-practice-uploader#70"
	}
	st := m.prStatus[pr.Code]
	label := ref
	if st.Title != "" {
		label += " " + st.Title
	}
	line := mdLink(label, prURL(pr.Repo, pr.Code))
	if st.Title != "" || st.Additions != 0 || st.Deletions != 0 {
		line += fmt.Sprintf(" `+%d/-%d`", st.Additions, st.Deletions)
	}
	return line
}

// mdLink renders "[label](url)" — a markdown link with the label as clickable
// text; just the URL when there's no label.
func mdLink(label, url string) string {
	if strings.TrimSpace(label) == "" {
		return url
	}
	return "[" + label + "](" + url + ")"
}

// jiraSummarySuffix is " summary" when the issue's summary is cached, else "".
func jiraSummarySuffix(st JiraStatus) string {
	if st.Title == "" {
		return ""
	}
	return " " + st.Title
}

// trackShareText is an event name followed by an inline-code object of its
// marks (the properties inside the event), so it pastes as a compact snippet.
func trackShareText(t model.Track) string {
	if len(t.Marks) == 0 {
		return t.Event
	}
	return t.Event + "\n  `{ " + strings.Join(t.Marks, ", ") + " }`"
}

// findTrack returns the quest's track with the given event (empty Track if
// none — trackShareText then just returns the event name).
func (m *Model) findTrack(q *model.Quest, event string) model.Track {
	for _, t := range q.Tracks {
		if t.Event == event {
			return t
		}
	}
	return model.Track{Event: event}
}

// freeURLRE matches a bare http(s) URL run in body text. Trailing sentence
// punctuation is trimmed off the match afterwards (trimURLTail).
var freeURLRE = regexp.MustCompile(`https?://[^\s]+`)

// shortenLen is a safety net: per-token truncation does the real work, so this
// only clips pathological URLs (very many segments) — set well above a normal
// shortened link so typical ones keep their full "…/edit?tab=…#heading=…" tail.
const shortenLen = 100

// shortenURL renders a compact display of a URL: scheme + "www." dropped, long
// opaque path tokens truncated to a prefix + "…", and query/fragment values
// collapsed to "…" while keeping their keys — e.g.
//
//	https://docs.google.com/document/d/1FmbgLVtTtBy…/edit?tab=…#heading=…
func shortenURL(raw string) string {
	s := raw
	for _, p := range []string{"https://", "http://"} {
		s = strings.TrimPrefix(s, p)
	}
	s = strings.TrimPrefix(s, "www.")

	frag := ""
	if i := strings.Index(s, "#"); i >= 0 {
		frag, s = s[i+1:], s[:i]
	}
	query := ""
	if i := strings.Index(s, "?"); i >= 0 {
		query, s = s[i+1:], s[:i]
	}

	// Path: keep the host (segment 0) whole, truncate long later segments.
	parts := strings.Split(s, "/")
	for i := 1; i < len(parts); i++ {
		parts[i] = truncToken(parts[i], 12)
	}
	out := strings.Join(parts, "/")
	if query != "" {
		out += "?" + shortenParams(query)
	}
	if frag != "" {
		out += "#" + shortenParams(frag)
	}
	if r := []rune(out); len(r) > shortenLen {
		out = string(r[:shortenLen-1]) + "…"
	}
	return out
}

// truncToken shortens an opaque token to keep+"…" when it's longer than a small
// threshold; short, meaningful segments (like "edit", "d") pass through.
func truncToken(tok string, keep int) string {
	r := []rune(tok)
	if len(r) <= keep+2 {
		return tok
	}
	return string(r[:keep]) + "…"
}

// shortenParams collapses each "key=value" to "key=…" (values are the noisy
// part), keeping the keys as a hint of what the link points at.
func shortenParams(q string) string {
	parts := strings.Split(q, "&")
	for i, p := range parts {
		if k, _, ok := strings.Cut(p, "="); ok && k != "" {
			parts[i] = k + "=…"
		}
	}
	return strings.Join(parts, "&")
}

// trimURLTail drops trailing sentence punctuation a URL regex greedily grabs.
func trimURLTail(u string) string {
	return strings.TrimRight(u, ".,);:!?")
}

// shortenBodyLinks replaces every bare URL left in text (those NOT captured to
// a sigil) with a compact display, recording display→full in q.BodyLinks so the
// link stays clickable/copyable. Returns the rewritten text and whether any URL
// was shortened (so a caller can avoid a needless reseed — and not eat a
// just-typed trailing space — when nothing changed).
func (m *Model) shortenBodyLinks(q *model.Quest, text string) (string, bool) {
	return shortenLinksInto(&q.BodyLinks, text)
}

// shortenLinksInto replaces every bare URL in text with a compact display,
// recording display→full in *links (allocating the map on first use) so the
// full address stays recoverable. Returns the rewritten text and whether any
// URL changed. Shared by quest bodies and campaign notes.
func shortenLinksInto(links *map[string]string, text string) (string, bool) {
	changed := false
	out := freeURLRE.ReplaceAllStringFunc(text, func(match string) string {
		changed = true
		url := trimURLTail(match)
		tail := match[len(url):] // punctuation to preserve after the link
		short := shortenURL(url)
		if *links == nil {
			*links = map[string]string{}
		}
		(*links)[short] = url
		return short + tail
	})
	return out, changed
}

// dominantRepo is the repo shared by most of a quest's PRs — the baseline
// against which a PR is judged "cross-repo" for ref shortening.
func dominantRepo(prs []model.PRLink) string {
	counts := map[string]int{}
	best, bestN := "", 0
	for _, pr := range prs {
		counts[pr.Repo]++
		if counts[pr.Repo] > bestN {
			best, bestN = pr.Repo, counts[pr.Repo]
		}
	}
	return best
}
