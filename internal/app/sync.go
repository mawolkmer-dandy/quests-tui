package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Integration sync (see the design in internal/app/links.go). Two in-memory
// caches on Model, keyed by code, hold the latest fetched status for every
// linked PR and Jira issue. They're NOT persisted and NOT part of undo — a
// relaunch just re-fetches. A ticker collects the distinct codes across all
// quests and fires the fetches off the UI goroutine, mirroring transition.go's
// transTick and app.go's waitForQuickAdd goroutine pattern.

// PRStatus is a pull request's CI + review-thread state.
type PRStatus struct {
	Code               string // "#47477"
	Status             string // "running" | "error" | "success"
	CommentsUnresolved int
	CommentsTotal      int
}

// JiraStatus is a Jira issue's coarse status category.
type JiraStatus struct {
	Code   string // "EPDCHAIR-5713"
	Status string // "todo" | "in progress" | "done"
}

// syncTarget is one quest's linked codes, collected for a sync pass.
type syncTarget struct {
	prCode   string
	prRepo   string
	jiraCode string
}

const syncFetchTimeout = 15 * time.Second

type syncTickMsg struct{}

type syncResultMsg struct {
	prs  []PRStatus
	jira []JiraStatus
}

// syncTick schedules the next sync pass, mirroring transTick.
func syncTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg { return syncTickMsg{} })
}

// onSyncTick fires a fetch pass (unless one is already in flight, or there's
// nothing to fetch) and re-arms the ticker either way, so the refresh loop
// keeps running regardless.
func (m *Model) onSyncTick() tea.Cmd {
	rearm := syncTick(m.syncInterval)
	if m.syncing {
		return rearm
	}
	prs, jira := m.collectSyncTargets()
	if len(prs) == 0 && len(jira) == 0 {
		return rearm
	}
	m.syncing = true
	return tea.Batch(rearm, runSync(prs, jira))
}

// applySyncResult stores a completed pass's results into the caches. It never
// calls save() — the caches aren't persisted.
func (m *Model) applySyncResult(msg syncResultMsg) {
	for _, st := range msg.prs {
		m.prStatus[st.Code] = st
	}
	for _, st := range msg.jira {
		m.jiraStatus[st.Code] = st
	}
	m.lastSyncAt = time.Now()
	m.syncing = false
}

// collectSyncTargets gathers the distinct PRs and Jira issues linked across
// all quests, so each is fetched at most once per pass.
func (m *Model) collectSyncTargets() (prs []syncTarget, jira []string) {
	seenPR := map[string]bool{}
	seenJira := map[string]bool{}
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		if q.PRCode != "" && q.PRRepo != "" && !seenPR[q.PRCode] {
			seenPR[q.PRCode] = true
			prs = append(prs, syncTarget{prCode: q.PRCode, prRepo: q.PRRepo})
		}
		if q.JiraCode != "" && !seenJira[q.JiraCode] {
			seenJira[q.JiraCode] = true
			jira = append(jira, q.JiraCode)
		}
	}
	return prs, jira
}

// runSync fetches every target's status off the UI goroutine and returns a
// single syncResultMsg. Errors on individual fetches drop that code from the
// result (leaving its cache untouched in the Update handler) rather than
// failing the whole pass.
func runSync(prs []syncTarget, jira []string) tea.Cmd {
	return func() tea.Msg {
		var res syncResultMsg
		for _, t := range prs {
			st, ok := fetchPRStatus(t.prCode, t.prRepo)
			if ok {
				res.prs = append(res.prs, st)
			}
		}
		for _, code := range jira {
			st, ok := fetchJiraStatus(code)
			if ok {
				res.jira = append(res.jira, st)
			}
		}
		return res
	}
}

// --- PR fetches -----------------------------------------------------------

// prRollupEntry is one entry of gh's statusCheckRollup array: CheckRun entries
// carry {status, conclusion}, StatusContext entries carry {state}.
type prRollupEntry struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

type prRollupResponse struct {
	StatusCheckRollup []prRollupEntry `json:"statusCheckRollup"`
}

type reviewThreadsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []struct {
						IsResolved bool `json:"isResolved"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

func fetchPRStatus(prCode, prRepo string) (PRStatus, bool) {
	num := strings.TrimPrefix(prCode, "#")
	owner, repo, ok := splitRepo(prRepo)
	if !ok {
		return PRStatus{}, false
	}

	status, ok := fetchPRCIStatus(prRepo, num)
	if !ok {
		return PRStatus{}, false
	}
	unresolved, total, ok := fetchPRReviewThreads(owner, repo, num)
	if !ok {
		return PRStatus{}, false
	}
	return PRStatus{Code: prCode, Status: status, CommentsUnresolved: unresolved, CommentsTotal: total}, true
}

func fetchPRCIStatus(prRepo, num string) (string, bool) {
	url := prURL(prRepo, num)
	out, err := runCmd("gh", "pr", "view", url, "--json", "statusCheckRollup")
	if err != nil {
		return "", false
	}
	var resp prRollupResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", false
	}
	return collapseRollup(resp.StatusCheckRollup), true
}

// collapseRollup reduces gh's mixed CheckRun/StatusContext rollup to one of
// running/error/success: any in-flight check → running; else any failure →
// error; else success.
func collapseRollup(entries []prRollupEntry) string {
	running := map[string]bool{
		"queued": true, "in_progress": true, "pending": true,
		"waiting": true, "requested": true, "expected": true,
	}
	failure := map[string]bool{
		"failure": true, "error": true, "timed_out": true, "cancelled": true,
		"action_required": true, "startup_failure": true, "stale": true,
	}
	anyFailure := false
	for _, e := range entries {
		if running[strings.ToLower(e.Status)] || running[strings.ToLower(e.State)] {
			return "running"
		}
		if failure[strings.ToLower(e.Conclusion)] || failure[strings.ToLower(e.State)] {
			anyFailure = true
		}
	}
	if anyFailure {
		return "error"
	}
	return "success"
}

const reviewThreadsQuery = `query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$number){reviewThreads(first:100){nodes{isResolved}}}}}`

func fetchPRReviewThreads(owner, repo, num string) (unresolved, total int, ok bool) {
	out, err := runCmd("gh", "api", "graphql",
		"-f", "query="+reviewThreadsQuery,
		"-F", "owner="+owner,
		"-F", "repo="+repo,
		"-F", "number="+num,
	)
	if err != nil {
		return 0, 0, false
	}
	var resp reviewThreadsResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, 0, false
	}
	nodes := resp.Data.Repository.PullRequest.ReviewThreads.Nodes
	for _, n := range nodes {
		if !n.IsResolved {
			unresolved++
		}
	}
	return unresolved, len(nodes), true
}

// --- Jira fetch -----------------------------------------------------------

type jiraViewResponse struct {
	Fields struct {
		Status struct {
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
	} `json:"fields"`
}

func fetchJiraStatus(code string) (JiraStatus, bool) {
	out, err := runCmd("acli", "jira", "workitem", "view", code, "--json", "--fields", "status")
	if err != nil {
		return JiraStatus{}, false
	}
	var resp jiraViewResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return JiraStatus{}, false
	}
	status, ok := jiraCategoryStatus(resp.Fields.Status.StatusCategory.Key)
	if !ok {
		return JiraStatus{}, false
	}
	return JiraStatus{Code: code, Status: status}, true
}

// jiraCategoryStatus maps Jira's statusCategory key to the coarse label shown
// in the UI. An unrecognized key is treated as unknown (leaving the code
// unsynced, so it keeps showing the loading dot).
func jiraCategoryStatus(key string) (string, bool) {
	switch key {
	case "new":
		return "todo", true
	case "indeterminate":
		return "in progress", true
	case "done":
		return "done", true
	}
	return "", false
}

// --- helpers --------------------------------------------------------------

func splitRepo(repo string) (owner, name string, ok bool) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// runCmd runs an external command with a bounded timeout and returns its
// stdout. Used for the gh/acli fetches — kept tiny so each call site stays
// declarative.
func runCmd(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), syncFetchTimeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// --- rendering ------------------------------------------------------------

// jiraGlyph is the filling-circle status glyph for a Jira code: the loading
// dot until its sync lands, then empty/half/full for todo/in progress/done.
func (m *Model) jiraGlyph(code string) string {
	st, ok := m.jiraStatus[code]
	if !ok {
		return ui.StyleMuted.Render(ui.GlyphLoading)
	}
	switch st.Status {
	case "done":
		return lipgloss.NewStyle().Foreground(ui.ColorHeading).Render(ui.GlyphJiraDone)
	case "in progress":
		return lipgloss.NewStyle().Foreground(ui.ColorPriorityMedium).Render(ui.GlyphJiraInProgress)
	default: // todo
		return ui.StyleMuted.Render(ui.GlyphJiraTodo)
	}
}

// prGlyph is the CI status glyph for a PR code: loading dot until synced, then
// check/cross/dotted-circle for success/error/running.
func (m *Model) prGlyph(code string) (glyph string, synced bool) {
	st, ok := m.prStatus[code]
	if !ok {
		return ui.StyleMuted.Render(ui.GlyphLoading), false
	}
	switch st.Status {
	case "error":
		return lipgloss.NewStyle().Foreground(ui.ColorImportant).Render(ui.GlyphPRError), true
	case "running":
		return ui.StyleRunning.Render(ui.GlyphPRRunning), true
	default: // success
		return lipgloss.NewStyle().Foreground(ui.ColorHeading).Render(ui.GlyphPRSuccess), true
	}
}

// integrationSegment is one code chunk (Jira or PR) of the meta line: its
// visible text, display width, and the URL a click on the code opens.
type integrationSegment struct {
	text  string // rendered (styled) text
	width int    // display width for click hit-testing
	url   string
}

// integrationSegments builds the Jira group then the PR group for q's linked
// codes, in that order, each as one clickable segment. The code text is muted;
// the status glyph follows it. Only groups whose code is set are included.
func (m *Model) integrationSegments(q *model.Quest) []integrationSegment {
	var segs []integrationSegment
	if q.JiraCode != "" {
		text := ui.StyleMuted.Render(q.JiraCode) + " " + m.jiraGlyph(q.JiraCode)
		width := lipgloss.Width(q.JiraCode) + 1 + 1
		segs = append(segs, integrationSegment{text: text, width: width, url: jiraURL(q.JiraCode, m.jiraBaseURL)})
	}
	if q.PRCode != "" {
		glyph, synced := m.prGlyph(q.PRCode)
		text := ui.StyleMuted.Render(q.PRCode) + " " + glyph
		width := lipgloss.Width(q.PRCode) + 1 + 1
		if synced {
			if st := m.prStatus[q.PRCode]; st.CommentsTotal > 0 {
				comments := fmt.Sprintf(" %d/%d", st.CommentsUnresolved, st.CommentsTotal)
				text += ui.StyleMuted.Render(comments)
				width += lipgloss.Width(comments)
			}
		}
		segs = append(segs, integrationSegment{text: text, width: width, url: prURL(q.PRRepo, q.PRCode)})
	}
	return segs
}

// focusCodeLines renders the expanded quest focus view's integration codes,
// one per line (Jira then PR), indented to align with the body text (4 cols),
// and records each code's clickable span against its content line index
// (startLn is the content line the first code line is emitted at). Returns the
// rendered lines in order.
func (m *Model) focusCodeLines(q *model.Quest, startLn int) []string {
	const indent = 4 // body text starts 4 cols in (see focusTextWidth)
	segs := m.integrationSegments(q)
	lines := make([]string, 0, len(segs))
	for i, seg := range segs {
		x := m.focusLeftMargin + indent
		m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{
			line: startLn + i,
			x0:   x,
			x1:   x + seg.width,
			url:  seg.url,
		})
		lines = append(lines, strings.Repeat(" ", indent)+seg.text)
	}
	return lines
}

// renderQuestMetaLine renders the integration sub-line for a RowQuestMeta,
// indented to align under the quest's title, and returns the clickable code
// spans (absolute screen columns). Jira and PR groups are kept close together
// (two spaces apart).
func (m *Model) renderQuestMetaLine(row ui.Row, width int) (string, []codeSpan) {
	q := m.findQuest(row.QuestID)
	if q == nil {
		return "", nil
	}
	nestOffset := 0
	if row.Nested {
		nestOffset = 2
	}
	// Align under the quest title (see RenderRow's RowQuest layout /
	// titleOffset): cursor mark (2) + nest + priority slot (4) + glyph (1) +
	// space (1) = 8 + nest.
	indent := 8 + nestOffset
	segs := m.integrationSegments(q)
	if len(segs) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(strings.Repeat(" ", indent))
	x := m.leftMargin + indent
	var spans []codeSpan
	for i, seg := range segs {
		if i > 0 {
			b.WriteString("  ")
			x += 2
		}
		// Only the code text itself is clickable, not the trailing glyph /
		// counts — but hit-testing the whole segment is close enough and
		// simpler, so the span covers the segment's code+glyph extent.
		spans = append(spans, codeSpan{x0: x, x1: x + seg.width, url: seg.url})
		b.WriteString(seg.text)
		x += seg.width
	}
	return b.String(), spans
}
