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

// PRStatus is a pull request's CI + review-thread state, plus the branch refs
// used to order linked PRs into a Graphite-style stack (see prStack).
type PRStatus struct {
	Code               string // "#47477"
	Status             string // "running" | "error" | "success"
	CommentsUnresolved int
	CommentsTotal      int
	BaseRef            string // the branch this PR targets (baseRefName)
	HeadRef            string // this PR's own branch (headRefName)
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
// all quests, so each is fetched at most once per pass. Every quest's whole
// PRs slice is iterated (a quest can link several).
func (m *Model) collectSyncTargets() (prs []syncTarget, jira []string) {
	seenPR := map[string]bool{}
	seenJira := map[string]bool{}
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		for _, pr := range q.PRs {
			if pr.Code != "" && pr.Repo != "" && !seenPR[pr.Code] {
				seenPR[pr.Code] = true
				prs = append(prs, syncTarget{prCode: pr.Code, prRepo: pr.Repo})
			}
		}
		if q.JiraCode != "" && !seenJira[q.JiraCode] {
			seenJira[q.JiraCode] = true
			jira = append(jira, q.JiraCode)
		}
	}
	return prs, jira
}

// syncSubsetForCodes collects only the targets whose code is in codes, so a
// freshly-captured link can be fetched immediately without waiting for the
// next tick or re-fetching everything. A code already in cache is still
// re-fetched (its status may have moved), but the common case is one or two
// brand-new codes.
func (m *Model) syncSubsetForCodes(codes []string) (prs []syncTarget, jira []string) {
	want := map[string]bool{}
	for _, c := range codes {
		want[c] = true
	}
	allPRs, allJira := m.collectSyncTargets()
	for _, t := range allPRs {
		if want[t.prCode] {
			prs = append(prs, t)
		}
	}
	for _, c := range allJira {
		if want[c] {
			jira = append(jira, c)
		}
	}
	return prs, jira
}

// syncNow fires an immediate fetch pass for just the given codes, respecting
// the in-flight guard — if a pass is already running the codes simply keep
// showing the fetching glyph until it lands. Returns nil when there's nothing
// to do (guard held, no matching targets, or integrations off).
func (m *Model) syncNow(codes []string) tea.Cmd {
	if !m.integrationsEnabled || m.syncing || len(codes) == 0 {
		return nil
	}
	prs, jira := m.syncSubsetForCodes(codes)
	if len(prs) == 0 && len(jira) == 0 {
		return nil
	}
	m.syncing = true
	return runSync(prs, jira)
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
	BaseRefName       string          `json:"baseRefName"`
	HeadRefName       string          `json:"headRefName"`
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

	status, baseRef, headRef, ok := fetchPRCIStatus(prRepo, num)
	if !ok {
		return PRStatus{}, false
	}
	unresolved, total, ok := fetchPRReviewThreads(owner, repo, num)
	if !ok {
		return PRStatus{}, false
	}
	return PRStatus{
		Code:               prCode,
		Status:             status,
		CommentsUnresolved: unresolved,
		CommentsTotal:      total,
		BaseRef:            baseRef,
		HeadRef:            headRef,
	}, true
}

func fetchPRCIStatus(prRepo, num string) (status, baseRef, headRef string, ok bool) {
	url := prURL(prRepo, num)
	out, err := runCmd("gh", "pr", "view", url, "--json", "statusCheckRollup,baseRefName,headRefName")
	if err != nil {
		return "", "", "", false
	}
	var resp prRollupResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", "", false
	}
	return collapseRollup(resp.StatusCheckRollup), resp.BaseRefName, resp.HeadRefName, true
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

// --- stack ordering -------------------------------------------------------

// prStackNode is one linked PR positioned within a Graphite-style stack: its
// link, its tree depth (0 = a root targeting main/trunk or an unlinked
// branch), and whether it's the last child on its immediate branch (used to
// decide connector glyphs, though the current renderer draws a uniform
// connector).
type prStackNode struct {
	link  model.PRLink
	depth int
}

// prStack orders a quest's linked PRs into a Graphite-style stack using the
// fetched branch refs: a PR is a CHILD of another linked PR when its BaseRef
// equals that PR's HeadRef. Roots (PRs whose base is not the head of any other
// linked PR — i.e. they target main/trunk or a branch outside the set) come
// first, each followed immediately by its transitive children (depth+1 per
// level). Independent PRs are separate roots. The output is a flat, render-
// ready pre-order with depths.
//
// Ordering is stable and deterministic: roots keep their link order, and each
// node's children keep theirs, so an unsynced set (no refs yet) simply renders
// as a flat list of roots in link order.
func (m *Model) prStack(prs []model.PRLink) []prStackNode {
	// Index PRs by their head branch so a child can find its parent by base.
	headToIdx := map[string]int{}
	for i, pr := range prs {
		if st, ok := m.prStatus[pr.Code]; ok && st.HeadRef != "" {
			headToIdx[st.HeadRef] = i
		}
	}

	// parent[i] is the index of i's parent PR within prs, or -1 for a root.
	parent := make([]int, len(prs))
	children := make([][]int, len(prs))
	for i, pr := range prs {
		parent[i] = -1
		st, ok := m.prStatus[pr.Code]
		if !ok || st.BaseRef == "" {
			continue
		}
		if p, ok := headToIdx[st.BaseRef]; ok && p != i {
			parent[i] = p
		}
	}
	for i := range prs {
		if parent[i] >= 0 {
			children[parent[i]] = append(children[parent[i]], i)
		}
	}

	var nodes []prStackNode
	var visit func(i, depth int)
	visited := make([]bool, len(prs))
	visit = func(i, depth int) {
		if visited[i] {
			return // defend against a ref cycle
		}
		visited[i] = true
		nodes = append(nodes, prStackNode{link: prs[i], depth: depth})
		for _, c := range children[i] {
			visit(c, depth+1)
		}
	}
	for i := range prs {
		if parent[i] < 0 {
			visit(i, 0)
		}
	}
	// Any PR left unvisited (part of a cycle whose members all had parents)
	// still needs to render — append them flat as roots.
	for i := range prs {
		if !visited[i] {
			visit(i, 0)
		}
	}
	return nodes
}

// --- rendering ------------------------------------------------------------

// prStatusWord is the expanded-view word for a PR's CI state:
// success→"passing", error→"failing", running→"running", not-yet-synced→
// "fetching…".
func (m *Model) prStatusWord(code string) string {
	st, ok := m.prStatus[code]
	if !ok {
		return "fetching…"
	}
	switch st.Status {
	case "error":
		return "failing"
	case "running":
		return "running"
	default:
		return "passing"
	}
}

// jiraStatusWord is the expanded-view Title-cased word for a Jira issue's
// category: "To Do" / "In Progress" / "Done" (fetching→"fetching…").
func (m *Model) jiraStatusWord(code string) string {
	st, ok := m.jiraStatus[code]
	if !ok {
		return "fetching…"
	}
	switch st.Status {
	case "done":
		return "Done"
	case "in progress":
		return "In Progress"
	default:
		return "To Do"
	}
}

// prCommentsText is the always-shown "<unresolved>/<total> comments" for a PR,
// including "0/0" when there are none (or before it's synced).
func (m *Model) prCommentsText(code string) string {
	st := m.prStatus[code]
	return fmt.Sprintf("%d/%d comments", st.CommentsUnresolved, st.CommentsTotal)
}

// prCommentsCount is the compact "<unresolved>/<total>" for the list inline,
// always shown (0/0 included).
func (m *Model) prCommentsCount(code string) string {
	st := m.prStatus[code]
	return fmt.Sprintf("%d/%d", st.CommentsUnresolved, st.CommentsTotal)
}

// jiraGlyph is the filling-circle status glyph for a Jira code: the amber
// "fetching" dotted circle until its sync lands, then empty/half/full for
// todo/in progress/done.
func (m *Model) jiraGlyph(code string) string {
	st, ok := m.jiraStatus[code]
	if !ok {
		return ui.StyleRunning.Render(ui.GlyphFetching)
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

// prGlyph is the CI status glyph for a PR code: the amber "fetching" dotted
// circle until synced, then check/cross for success/error and the same amber
// dotted circle while CI is running.
func (m *Model) prGlyph(code string) (glyph string, synced bool) {
	st, ok := m.prStatus[code]
	if !ok {
		return ui.StyleRunning.Render(ui.GlyphFetching), false
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

// integrationSegments builds the Jira segment then one segment per linked PR
// (in stack order), each clickable. The code text is muted; its status glyph
// follows it; each PR also carries its always-shown "<u>/<t>" comment count
// (0/0 included). No tree/connectors here — that's the expanded view only.
func (m *Model) integrationSegments(q *model.Quest) []integrationSegment {
	var segs []integrationSegment
	if q.JiraCode != "" {
		text := ui.StyleMuted.Render(q.JiraCode) + " " + m.jiraGlyph(q.JiraCode)
		width := lipgloss.Width(q.JiraCode) + 1 + 1
		segs = append(segs, integrationSegment{text: text, width: width, url: jiraURL(q.JiraCode, m.jiraBaseURL)})
	}
	for _, node := range m.prStack(q.PRs) {
		pr := node.link
		glyph, _ := m.prGlyph(pr.Code)
		count := " " + m.prCommentsCount(pr.Code)
		text := ui.StyleMuted.Render(pr.Code) + " " + glyph + ui.StyleMuted.Render(count)
		width := lipgloss.Width(pr.Code) + 1 + 1 + lipgloss.Width(count)
		segs = append(segs, integrationSegment{text: text, width: width, url: prURL(pr.Repo, pr.Code)})
	}
	return segs
}

// focusCodeLines renders the expanded quest focus view's integration links,
// indented to align with the body text (4 cols): the Jira line first (plain),
// then the linked PRs drawn as a gt-ls-style vertical stack — parent on top, a
// muted "│" connector between consecutive PRs, each PR line prefixed by a muted
// "○" node then "#code  <status word> · <u/t> comments". A lone PR (no stack)
// renders as a plain PR line with no connector.
//
// It records each link's clickable span (focusCodeSpans, for mouse) AND its
// navigable focusLink entry (for the cursor), both keyed to the content-line
// index the line is emitted at (startLn is the first). When a link is the
// focused cursor target it also gets a muted action hint ("↵ open · Ctrl+X
// remove") or, while a removal is armed, the inline "remove this link? y/n"
// prompt — and its line index is recorded as the caret line for scrolling.
func (m *Model) focusCodeLines(q *model.Quest, startLn int) []string {
	const indent = 4 // body text starts 4 cols in (see focusTextWidth)
	pad := strings.Repeat(" ", indent)

	var lines []string
	ln := startLn

	// hintFor returns the trailing action hint / confirm prompt for the link at
	// focusLinks index li, when it's the focused cursor target.
	hintFor := func(li int) string {
		if m.focusLinkIdx != li {
			return ""
		}
		if m.focusLinkConfirmID != "" {
			return "  " + ui.StyleImportant.Render("remove this link? y/n")
		}
		return "  " + ui.StyleMuted.Render("↵ open · "+Keys.Delete.Help().Key+" remove")
	}

	addLink := func(content, prefix string, prefixW int, kind linkKind, code, url string) {
		li := len(m.focusLinks)
		x := m.focusLeftMargin + indent + prefixW
		// The clickable code span sits after the stack prefix.
		m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x, x1: x + lipgloss.Width(content), url: url})
		m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: kind, code: code, url: url})
		if m.focusLinkIdx == li {
			m.focusCaretLine = ln
		}
		lines = append(lines, pad+prefix+content+hintFor(li))
		ln++
	}

	if q.JiraCode != "" {
		content := ui.StyleMuted.Render(q.JiraCode) + " " + m.jiraGlyph(q.JiraCode) + " " + ui.StyleMuted.Render(m.jiraStatusWord(q.JiraCode))
		addLink(content, "", 0, linkJira, q.JiraCode, jiraURL(q.JiraCode, m.jiraBaseURL))
	}

	stack := m.prStack(q.PRs)
	for i, node := range stack {
		pr := node.link
		glyph, _ := m.prGlyph(pr.Code)
		content := ui.StyleMuted.Render(pr.Code) + "  " + glyph + " " +
			ui.StyleMuted.Render(m.prStatusWord(pr.Code)+" · "+m.prCommentsText(pr.Code))

		// A connector line above every PR after the first of a stack, so
		// consecutive PRs read as a single stack; a lone PR gets no connector.
		if i > 0 {
			lines = append(lines, pad+ui.StyleMuted.Render(ui.GlyphStackConnector))
			ln++
		}
		prefix := ui.StyleMuted.Render(ui.GlyphStackNode) + " "
		addLink(content, prefix, lipgloss.Width(ui.GlyphStackNode)+1, linkPR, pr.Code, prURL(pr.Repo, pr.Code))
	}
	return lines
}

// focusLinkCount is how many navigable link lines the expanded quest view
// currently has (Jira + each PR) — used to bound link-cursor movement.
func (m *Model) focusLinkCount(q *model.Quest) int {
	n := len(m.prStack(q.PRs))
	if q.JiraCode != "" {
		n++
	}
	return n
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
