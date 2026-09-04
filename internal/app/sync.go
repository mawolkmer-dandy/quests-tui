package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	Code             string // "#47477"
	Status           string // "running" | "error" | "success" | "merged" | "closed"
	CommentsResolved int
	CommentsTotal    int
	BaseRef          string    // the branch this PR targets (baseRefName)
	HeadRef          string    // this PR's own branch (headRefName)
	HeadSHA          string    // this PR's head commit oid — auto-find re-diffs only when it changes
	MergedAt         time.Time // when the PR merged (zero if not merged) — ages tracks/runes it introduced
	Title            string    // PR title, for the shareable copy-section list
	Additions        int       // lines added, for the "+N/-M" churn suffix
	Deletions        int       // lines removed
	Draft            bool      // draft PR — a distinct emoji in the copied list
}

// JiraStatus is a Jira issue's coarse status category.
type JiraStatus struct {
	Code   string // "EPDCHAIR-5713"
	Status string // "todo" | "in progress" | "done"
	Title  string // issue summary, for the shareable copy-section list
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
	prs   []PRStatus
	jira  []JiraStatus
	runes []RuneStatus
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
	runes := m.collectRuneKeys()
	if len(prs) == 0 && len(jira) == 0 && len(runes) == 0 {
		return rearm
	}
	m.syncing = true
	return tea.Batch(rearm, runSync(prs, jira, runes, m.ldProject, m.ldEnv), m.maybeStartSpinner())
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
	for _, st := range msg.runes {
		m.runeStatus[st.Key] = st
	}
	m.lastSyncAt = time.Now()
	m.syncing = false
	m.invalidateRender() // integration statuses changed → refresh cached content
}

// collectSyncTargets gathers the distinct PRs and Jira issues linked across
// all quests, so each is fetched at most once per pass. Every quest's whole
// PRs slice is iterated (a quest can link several).
func (m *Model) collectSyncTargets() (prs []syncTarget, jira []string) {
	seenPR := map[string]bool{}
	seenJira := map[string]bool{}
	for i := range m.store.Quests {
		q := &m.store.Quests[i]
		if m.isVaulted(q) {
			continue // vaulted quests are silent — keep their links, don't fetch
		}
		for _, pr := range q.PRs {
			if pr.Code != "" && pr.Repo != "" && !seenPR[pr.Code] {
				seenPR[pr.Code] = true
				prs = append(prs, syncTarget{prCode: pr.Code, prRepo: pr.Repo})
			}
		}
		for _, code := range q.JiraCodes {
			if !seenJira[code] {
				seenJira[code] = true
				jira = append(jira, code)
			}
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
	return runSync(prs, jira, nil, m.ldProject, m.ldEnv)
}

// runSync fetches every target's status off the UI goroutine and returns a
// single syncResultMsg. Errors on individual fetches drop that code from the
// result (leaving its cache untouched in the Update handler) rather than
// failing the whole pass.
func runSync(prs []syncTarget, jira []string, runes []string, ldProject, ldEnv string) tea.Cmd {
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
		for _, key := range runes {
			st, ok := fetchRuneStatus(ldProject, ldEnv, key)
			if ok {
				res.runes = append(res.runes, st)
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
	HeadRefOid        string          `json:"headRefOid"`
	MergedAt          string          `json:"mergedAt"` // RFC3339, empty if not merged
	State             string          `json:"state"`    // OPEN | MERGED | CLOSED
	Title             string          `json:"title"`
	Additions         int             `json:"additions"`
	Deletions         int             `json:"deletions"`
	IsDraft           bool            `json:"isDraft"`
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

	status, resp, ok := fetchPRCIStatus(prRepo, num)
	if !ok {
		return PRStatus{}, false
	}
	resolved, total, ok := fetchPRReviewThreads(owner, repo, num)
	if !ok {
		return PRStatus{}, false
	}
	st := PRStatus{
		Code:             prCode,
		Status:           status,
		CommentsResolved: resolved,
		CommentsTotal:    total,
		BaseRef:          resp.BaseRefName,
		HeadRef:          resp.HeadRefName,
		HeadSHA:          resp.HeadRefOid,
		Title:            resp.Title,
		Additions:        resp.Additions,
		Deletions:        resp.Deletions,
		Draft:            resp.IsDraft,
	}
	if resp.MergedAt != "" {
		if t, err := time.Parse(time.RFC3339, resp.MergedAt); err == nil {
			st.MergedAt = t
		}
	}
	return st, true
}

func fetchPRCIStatus(prRepo, num string) (status string, resp prRollupResponse, ok bool) {
	url := prURL(prRepo, num)
	out, err := runCmd("gh", "pr", "view", url, "--json", "state,statusCheckRollup,baseRefName,headRefName,headRefOid,mergedAt,title,additions,deletions,isDraft")
	if err != nil {
		return "", prRollupResponse{}, false
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", prRollupResponse{}, false
	}
	// A merged/closed PR outranks its last CI run — a merged PR showing
	// "passing" (or a closed one showing whatever its checks were) would read
	// as still-open work.
	switch strings.ToUpper(resp.State) {
	case "MERGED":
		return "merged", resp, true
	case "CLOSED":
		return "closed", resp, true
	}
	return collapseRollup(resp.StatusCheckRollup), resp, true
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

func fetchPRReviewThreads(owner, repo, num string) (resolved, total int, ok bool) {
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
		if n.IsResolved {
			resolved++
		}
	}
	return resolved, len(nodes), true
}

// --- Jira fetch -----------------------------------------------------------

type jiraViewResponse struct {
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
	} `json:"fields"`
}

func fetchJiraStatus(code string) (JiraStatus, bool) {
	out, err := runCmd("acli", "jira", "workitem", "view", code, "--json", "--fields", "status,summary")
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
	return JiraStatus{Code: code, Status: status, Title: resp.Fields.Summary}, true
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
	return runCmdTimeout(syncFetchTimeout, name, args...)
}

// runCmdTimeout is runCmd with a caller-chosen timeout — used by the PR
// harvest, where a large `gh pr diff` can outrun the short sync timeout.
func runCmdTimeout(d time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// --- stack ordering -------------------------------------------------------

// prStackNode is one linked PR positioned within a Graphite-style stack: its
// link, its tree depth (0 = a root targeting main/trunk or an unlinked
// branch), and whether it belongs to a real multi-PR stack (a connected
// component of size > 1). stacked gates the tree connector glyphs, so two
// unrelated PRs on the same quest don't render as if they were stacked.
type prStackNode struct {
	link    model.PRLink
	depth   int
	stacked bool
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

	visited := make([]bool, len(prs))
	var visit func(i, depth int, out *[]prStackNode)
	visit = func(i, depth int, out *[]prStackNode) {
		if visited[i] {
			return // defend against a ref cycle
		}
		visited[i] = true
		*out = append(*out, prStackNode{link: prs[i], depth: depth})
		for _, c := range children[i] {
			visit(c, depth+1, out)
		}
	}
	// Build one connected component per root, then flag every node in a
	// multi-PR component as stacked. Components render contiguously (a root
	// followed by its descendants), so this keeps the pre-order output.
	var nodes []prStackNode
	appendComponent := func(root int) {
		var comp []prStackNode
		visit(root, 0, &comp)
		stacked := len(comp) > 1
		for k := range comp {
			comp[k].stacked = stacked
		}
		nodes = append(nodes, comp...)
	}
	for i := range prs {
		if parent[i] < 0 {
			appendComponent(i)
		}
	}
	// Any PR left unvisited (part of a cycle whose members all had parents)
	// still needs to render — append each as its own component root.
	for i := range prs {
		if !visited[i] {
			appendComponent(i)
		}
	}
	return nodes
}

// --- rendering ------------------------------------------------------------

// prStatusWord is the expanded-view word for a PR's state: merged→"merged",
// closed→"closed", else the CI state success→"passing", error→"failing",
// running→"running" (not-yet-synced→"fetching…").
func (m *Model) prStatusWord(code string) string {
	st, ok := m.prStatus[code]
	if !ok {
		return "fetching…"
	}
	switch st.Status {
	case "merged":
		return "merged"
	case "closed":
		return "closed"
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

// prCommentsText is the always-shown "<resolved>/<total> comments" for a PR,
// including "0/0" when there are none (or before it's synced). A PR is fully
// addressed when the two numbers match.
func (m *Model) prCommentsText(code string) string {
	st := m.prStatus[code]
	return fmt.Sprintf("%d/%d comments", st.CommentsResolved, st.CommentsTotal)
}

// prCommentsCount is the compact "<resolved>/<total>" for the list inline,
// always shown (0/0 included).
func (m *Model) prCommentsCount(code string) string {
	st := m.prStatus[code]
	return fmt.Sprintf("%d/%d", st.CommentsResolved, st.CommentsTotal)
}

// jiraGlyph is the filling-circle status glyph for a Jira code: a pulsing amber
// "fetching" dot until its sync lands, then empty/half/full for
// todo/in progress/done.
func (m *Model) jiraGlyph(code string) string {
	st, ok := m.jiraStatus[code]
	if !ok {
		return m.pulseStyle().Render(ui.GlyphFetching)
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

// prGlyph is the CI status glyph for a PR code: a pulsing amber dot while
// "fetching" (awaiting first sync), a pulsing amber circle while CI is
// running, then check/cross/merged/closed once it settles.
func (m *Model) prGlyph(code string) (glyph string, synced bool) {
	st, ok := m.prStatus[code]
	if !ok {
		return m.pulseStyle().Render(ui.GlyphFetching), false
	}
	switch st.Status {
	case "merged":
		return ui.StyleMerged.Render(ui.GlyphPRMerged), true
	case "closed":
		return ui.StyleMuted.Render(ui.GlyphPRClosed), true
	case "error":
		return lipgloss.NewStyle().Foreground(ui.ColorImportant).Render(ui.GlyphPRError), true
	case "running":
		return m.pulseStyle().Render(ui.GlyphPRRunning), true
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

// integrationSegments builds one segment per linked Jira issue then one per
// linked PR (in stack order), each clickable. The code text is muted; its
// status glyph follows it; each PR also carries its always-shown "<u>/<t>"
// comment count (0/0 included). No tree/connectors here — that's the expanded
// view only.
func (m *Model) integrationSegments(q *model.Quest) []integrationSegment {
	var segs []integrationSegment
	// Agent state comes first, as just its icon (managed from the expanded
	// view, so no click URL here).
	for _, id := range q.AgentWorkspaces {
		segs = append(segs, integrationSegment{text: m.agentGlyph(m.agentState(id)), width: 1})
	}
	for _, code := range q.JiraCodes {
		text := ui.StyleMuted.Render(code) + " " + m.jiraGlyph(code)
		width := lipgloss.Width(code) + 1 + 1
		segs = append(segs, integrationSegment{text: text, width: width, url: jiraURL(code, m.jiraBaseURL)})
	}
	for _, node := range m.prStack(q.PRs) {
		pr := node.link
		glyph, _ := m.prGlyph(pr.Code)
		count := " " + m.prCommentsCount(pr.Code)
		text := ui.StyleMuted.Render(pr.Code) + " " + glyph + ui.StyleMuted.Render(count)
		width := lipgloss.Width(pr.Code) + 1 + 1 + lipgloss.Width(count)
		segs = append(segs, integrationSegment{text: text, width: width, url: prURL(pr.Repo, pr.Code)})
	}
	// Attached runes (LaunchDarkly flags) — just the state icon inline.
	for _, key := range q.Runes {
		segs = append(segs, integrationSegment{text: m.runeGlyph(key), width: 1})
	}
	return segs
}

// focusCodeLines renders the expanded quest focus view's integration links,
// indented to align with the body text (4 cols). Every line shares one column
// layout so the status icons line up vertically:
//
//	<2-col stack gutter><status glyph> <code padded> <status text>
//
// The Jira line comes first (blank gutter), then the linked PRs in gt-ls stack
// order (parent targeting main on top). When 2+ PRs form a stack their gutter
// carries a muted tree marker — "├" for every PR but the last, "└" for the last
// — so they read as one connected stack; a lone PR gets a blank gutter. The
// code column is padded to the widest code so the trailing status text aligns
// too.
//
// It records each link's clickable span (focusCodeSpans, for mouse) AND its
// navigable focusLink entry (for the cursor), both keyed to the content-line
// index the line is emitted at (startLn is the first). When a link is the
// focused cursor target it also gets a muted action hint ("↵ open · Ctrl+X
// remove") or, while a removal is armed, the inline "remove this link? y/n"
// prompt — and its line index is recorded as the caret line for scrolling.
// questCampaignName is the quest's parent campaign name (its "parent"), or
// "Questboard" when it has no campaign yet.
func (m *Model) questCampaignName(q *model.Quest) string {
	if q.ProjectID == "" {
		return "Questboard"
	}
	if p := m.findProject(q.ProjectID); p != nil {
		return p.Name
	}
	return "—"
}

func questTypeLabel(q *model.Quest) string {
	if q.Type == model.QuestTypeMain {
		return "main quest"
	}
	return "side quest"
}

func (m *Model) questStatusLabel(q *model.Quest) string {
	s := "open"
	switch q.Status {
	case model.StatusActive:
		s = "active"
	case model.StatusDone:
		s = "done"
	}
	if m.isVaulted(q) {
		s += " · vaulted"
	}
	return s
}

func questPriorityLabel(q *model.Quest) string {
	switch q.Priority {
	case model.PriorityHigh:
		return "high"
	case model.PriorityMedium:
		return "medium"
	case model.PriorityLow:
		return "low"
	}
	return ""
}

func (m *Model) focusCodeLines(q *model.Quest, startLn, baseX int) []string {
	const gutterW = 2 // left slot holding the stack marker (blank otherwise)
	pad := ""         // detail-column lines are placed at baseX by the composer

	stack := m.prStack(q.PRs)

	// Pad every code to the widest one so the status text after it lines up.
	codeW := 0
	for _, code := range q.JiraCodes {
		if w := lipgloss.Width(code); w > codeW {
			codeW = w
		}
	}
	for _, node := range stack {
		if w := lipgloss.Width(node.link.Code); w > codeW {
			codeW = w
		}
	}

	var lines []string
	ln := startLn

	// hintFor returns the trailing action hint / confirm prompt for the link at
	// focusLinks index li, when it's the focused cursor target. The hint depends
	// on the link kind: browser links open + remove, the agent affordance adds,
	// a pinned agent only removes (no open — it's status-only).
	// The focused item's actions render on a fixed status line below the box
	// (see viewQuestDetail's sigilStatusLine), NOT inline — an inline hint made
	// long rows wrap and shifted the whole layout. hintFor is kept as a no-op so
	// the section loops read the same; drop it if the layout is ever reworked.
	hintFor := func(li int, kind linkKind) string { return "" }

	// addLink emits one aligned link line: a fixed-width stack gutter, the
	// (already-styled) status glyph, the padded code, then the status text. The
	// clickable span and cursor target both start at the code.
	// focusGutter is the 2-col left gutter for a sigil line: the accent "› "
	// cursor mark when this link is focused (matching the body/outline), else
	// the provided fallback (a stack marker or blank).
	focusGutter := func(li int, fallback string) string {
		if m.onFocusLink() && m.focusLinkIdx == li {
			return ui.StyleCursor.Render(ui.GlyphCursor)
		}
		return ui.StyleMuted.Render(fallback + strings.Repeat(" ", gutterW-lipgloss.Width(fallback)))
	}

	addLink := func(marker, glyph, code, text string, kind linkKind, url string) {
		li := len(m.focusLinks)
		x := baseX + gutterW + lipgloss.Width(glyph) + 1
		codePadded := code + strings.Repeat(" ", codeW-lipgloss.Width(code))
		// The code (Jira/PR id) reads white like the NPC label and rune keys;
		// only the trailing status word is muted.
		body := ui.StyleName.Render(codePadded) + "  " + text
		m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x, x1: x + lipgloss.Width(body), url: url})
		m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: kind, code: code, url: url})
		if m.focusLinkIdx == li {
			m.focusCaretLine = ln
		}
		lines = append(lines, pad+focusGutter(li, marker)+glyph+" "+body+hintFor(li, kind))
		ln++
	}

	agentPrefix := pad + strings.Repeat(" ", gutterW)
	// linePrefix is focusGutter for the sigil loops that don't go through
	// addLink (agents, runes, tracks, lookouts, affordances) — a "› " when
	// focused, else the blank agent-aligned gutter.
	linePrefix := func(li int) string {
		return pad + focusGutter(li, "")
	}

	// Metadata rows (non-navigable) at the top of the details column, indented
	// (agentPrefix) so their labels line up with the connection sections below.
	meta := func(label, value string) {
		if value == "" {
			return
		}
		lines = append(lines, agentPrefix+ui.StyleMuted.Render(fmt.Sprintf("%-10s", label))+value)
		ln++
	}
	meta("Campaign", m.questCampaignName(q))
	meta("Type", questTypeLabel(q))
	meta("Status", m.questStatusLabel(q))
	meta("Priority", questPriorityLabel(q))
	meta("Created", agoStr(q.CreatedAt))
	meta("Updated", agoStr(q.UpdatedAt))

	// sectionHeader emits a blank spacer then a muted "emblem Name" line. When
	// the section has items (count>0) the header is a focus stop that copies the
	// whole section as a shareable list ("c"); empty sections stay non-navigable.
	sectionHeader := func(glyph, name, sectionKey string, count int) {
		lines = append(lines, "") // spacer (non-navigable)
		ln++
		hdr := ui.StyleMuted.Render(glyph + " " + name)
		if count == 0 {
			lines = append(lines, agentPrefix+hdr)
			ln++
			return
		}
		li := len(m.focusLinks)
		m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: linkCopySection, code: sectionKey})
		if m.focusLinkIdx == li {
			m.focusCaretLine = ln
		}
		x0 := baseX + gutterW
		m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x0, x1: x0 + lipgloss.Width(glyph+" "+name), url: copySectionSentinel + sectionKey})
		lines = append(lines, linePrefix(li)+hdr+hintFor(li, linkCopySection))
		ln++
	}
	// NPCs (pinned agents).
	sectionHeader(ui.GlyphConnNPC, "NPCs", secNPCs, len(q.AgentWorkspaces))
	for _, id := range q.AgentWorkspaces {
		li := len(m.focusLinks)
		state := m.agentState(id)
		glyph := m.agentGlyph(state)
		m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: linkAgent, code: id})
		if m.focusLinkIdx == li {
			m.focusCaretLine = ln
		}
		// Clickable span over the name+status, so a click focuses the agent
		// exactly as Enter does (see handleFocusClick's agentFocusPrefix case).
		body := m.agentLabel(id) + "  " + ui.StyleMuted.Render(agentWord(state))
		x := baseX + gutterW + lipgloss.Width(glyph) + 1
		m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x, x1: x + lipgloss.Width(body), url: agentFocusPrefix + id})
		lines = append(lines, linePrefix(li)+glyph+" "+body+hintFor(li, linkAgent))
		ln++
	}
	if len(q.AgentWorkspaces) == 0 {
		li := len(m.focusLinks)
		m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: linkAddAgent})
		if m.focusLinkIdx == li {
			m.focusCaretLine = ln
		}
		label := "+ select a Herdr agent"
		x0 := baseX + gutterW + 2 // align with the paste-link affordances
		m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x0, x1: x0 + lipgloss.Width(label), url: addAgentSentinel})
		lines = append(lines, linePrefix(li)+"  "+ui.StyleMuted.Render(label)+hintFor(li, linkAddAgent))
		ln++
	}

	// Scrolls (Jira) — hidden when empty; paste a Jira link into the body to add.
	if len(q.JiraCodes) > 0 || m.showHiddenSigils {
		sectionHeader(ui.GlyphConnScroll, "Scrolls", secScrolls, len(q.JiraCodes))
		for _, code := range q.JiraCodes {
			text := ui.StyleMuted.Render(m.jiraStatusWord(code))
			addLink("", m.jiraGlyph(code), code, text, linkJira, jiraURL(code, m.jiraBaseURL))
		}
	}

	// Trails (GitHub PRs) — hidden when empty; paste a PR link into the body to add.
	if len(stack) > 0 || m.showHiddenSigils {
		sectionHeader(ui.GlyphConnTrail, "Trails", secTrails, len(stack))
		for i, node := range stack {
			pr := node.link
			glyph, _ := m.prGlyph(pr.Code)
			text := ui.StyleMuted.Render(m.prStatusWord(pr.Code) + " · " + m.prCommentsText(pr.Code))
			marker := ""
			if node.stacked {
				marker = ui.GlyphStackBranchMid
				if i == len(stack)-1 || stack[i+1].depth == 0 {
					marker = ui.GlyphStackBranchEnd
				}
			}
			addLink(marker, glyph, pr.Code, text, linkPR, prURL(pr.Repo, pr.Code))
		}
		if len(stack) > 0 {
			// Find affordance — scan the Trails for Tracks (events) + flags/issues.
			li := len(m.focusLinks)
			m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: linkFind})
			if m.focusLinkIdx == li {
				m.focusCaretLine = ln
			}
			label := ui.GlyphFind + " find tracks in trails"
			x0 := baseX + gutterW + 2
			m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x0, x1: x0 + lipgloss.Width(label), url: findSentinel})
			lines = append(lines, linePrefix(li)+"  "+ui.StyleMuted.Render(label)+hintFor(li, linkFind))
			ln++
		}
	}

	// Runes (LaunchDarkly flags) — hidden when empty; found from Trails.
	if len(q.Runes) > 0 || m.showHiddenSigils {
		sectionHeader(ui.GlyphConnRune, "Runes", secRunes, len(q.Runes))
		for _, key := range q.Runes {
			li := len(m.focusLinks)
			url := ldFlagURL(m.ldProject, m.ldEnv, key)
			glyph := m.runeGlyph(key)
			m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: linkRune, code: key, url: url})
			if m.focusLinkIdx == li {
				m.focusCaretLine = ln
			}
			body := ui.StyleName.Render(key) + "  " + ui.StyleMuted.Render(m.runeWord(key)) + ageDaysLabel(m.runeAgeDays(q.ID, key), true)
			// Register the clickable span (the rune loop is bespoke — no codeW
			// padding — so it can't use addLink; without this a click did nothing).
			x := baseX + gutterW + lipgloss.Width(glyph) + 1
			m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x, x1: x + lipgloss.Width(body), url: url})
			lines = append(lines, linePrefix(li)+glyph+" "+body+hintFor(li, linkRune))
			ln++
		}
	}

	// Tracks (found from the Trails — no manual entry) — hidden when empty unless
	// there are dismissed tracks to restore. The glyph is colored by whether the
	// event's source PR is merged (in production) or still pending.
	if len(q.Tracks) > 0 || len(q.DismissedTracks) > 0 || m.showHiddenSigils {
		sectionHeader(ui.GlyphConnTrack, "Tracks", secTracks, len(q.Tracks))
		for _, t := range q.Tracks {
			li := len(m.focusLinks)
			glyph := m.trackGlyph(t)
			m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: linkTrack, code: t.Event})
			if m.focusLinkIdx == li {
				m.focusCaretLine = ln
			}
			body := ui.StyleName.Render(t.Event) + "  " + ui.StyleMuted.Render(m.trackWord(t))
			x := baseX + gutterW + lipgloss.Width(glyph) + 1
			m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x, x1: x + lipgloss.Width(body), url: copyTrackSentinel + t.Event})
			lines = append(lines, linePrefix(li)+glyph+" "+body+hintFor(li, linkTrack))
			ln++
		}
		if len(q.Tracks) > 0 {
			// Write the Lookout's plans (a per-quest dashboard build-plan).
			li := len(m.focusLinks)
			m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: linkForge})
			if m.focusLinkIdx == li {
				m.focusCaretLine = ln
			}
			label := ui.GlyphVine + " write the Lookout's plans"
			x0 := baseX + gutterW + 2
			m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x0, x1: x0 + lipgloss.Width(label), url: forgeSentinel})
			lines = append(lines, linePrefix(li)+"  "+ui.StyleMuted.Render(label)+hintFor(li, linkForge))
			ln++
		}
		// Restore affordance — bring back tracks dismissed by mistake.
		if len(q.DismissedTracks) > 0 {
			li := len(m.focusLinks)
			m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: linkRestore})
			if m.focusLinkIdx == li {
				m.focusCaretLine = ln
			}
			label := fmt.Sprintf("⌀ %d dismissed", len(q.DismissedTracks))
			x0 := baseX + gutterW + 2
			m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x0, x1: x0 + lipgloss.Width(label), url: restoreSentinel})
			lines = append(lines, linePrefix(li)+"  "+ui.StyleMuted.Render(label)+hintFor(li, linkRestore))
			ln++
		}
	}

	// Lookouts (usage dashboards) — hidden when empty; paste a dashboard URL to add.
	if len(q.Lookouts) > 0 || m.showHiddenSigils {
		sectionHeader(ui.GlyphConnLookout, "Lookouts", secLookouts, len(q.Lookouts))
		for _, l := range q.Lookouts {
			li := len(m.focusLinks)
			glyph := lookoutGlyph()
			m.focusLinks = append(m.focusLinks, focusLink{line: ln, kind: linkLookout, code: l.URL, url: l.URL})
			if m.focusLinkIdx == li {
				m.focusCaretLine = ln
			}
			x := baseX + gutterW + lipgloss.Width(glyph) + 1
			if m.lookoutEditor != nil && m.lookoutEditURL == l.URL {
				// Inline rename in progress — render the editor in place of the label.
				lines = append(lines, linePrefix(li)+glyph+" "+m.renderEditableStyled(m.lookoutEditor, ui.StyleName))
				ln++
				continue
			}
			body := ui.StyleName.Render(lookoutLabel(l)) + ageDaysLabel(daysSince(l.AddedAt), false)
			m.focusCodeSpans = append(m.focusCodeSpans, focusCodeSpan{line: ln, x0: x, x1: x + lipgloss.Width(body), url: l.URL})
			lines = append(lines, linePrefix(li)+glyph+" "+body+hintFor(li, linkLookout))
			ln++
		}
	}

	// A muted, non-navigable hint to reveal/hide the empty connection sections
	// (F3). Only shown when there's actually something hidden to reveal.
	if m.sigilsHaveHidden(q) {
		label := "＋ show hidden sigils"
		if m.showHiddenSigils {
			label = "－ hide empty sigils"
		}
		lines = append(lines, "")
		lines = append(lines, agentPrefix+ui.StyleMuted.Render(label+"  F3"))
		ln += 2
	}
	return lines
}

// sigilsHaveHidden reports whether any hideable connection section is currently
// empty — i.e. there's something for Ctrl+E to reveal. NPCs is excluded: it
// always shows (its picker is the only way to add an agent).
func (m *Model) sigilsHaveHidden(q *model.Quest) bool {
	return len(q.JiraCodes) == 0 || len(m.prStack(q.PRs)) == 0 || len(q.Runes) == 0 ||
		(len(q.Tracks) == 0 && len(q.DismissedTracks) == 0) || len(q.Lookouts) == 0
}

// addAgentSentinel is a fake span URL marking the "+ add Claude agent" line, so
// a mouse click there opens the agent picker instead of a browser (see
// handleFocusMouse).
const addAgentSentinel = "\x00add-agent"

// forgeSentinel marks the "write the Lookout's plans" line; findSentinel marks
// the "find tracks in trails" line; restoreSentinel marks the "N dismissed"
// line — so a click there writes the plans / finds / restores, rather than
// opening a browser.
const forgeSentinel = "\x00write-plans"
const findSentinel = "\x00find-tracks"
const restoreSentinel = "\x00restore-tracks"

// agentFocusPrefix marks a pinned-agent line's clickable span; the herdr
// terminal id follows the prefix, so a click focuses that agent (matching
// Enter — see handleFocusClick).
const agentFocusPrefix = "\x00agent-focus:"

// copySectionSentinel marks a section-header's clickable span; the section key
// follows the prefix, so a click copies that whole section as a list.
const copySectionSentinel = "\x00copy-section:"

// copyTrackSentinel marks a track's clickable span; the event name follows the
// prefix, so a click copies the event + its marks (tracks have no URL to open).
const copyTrackSentinel = "\x00copy-track:"

// focusLinkCount is how many navigable link lines the expanded quest view
// currently has — the "Connections" master toggle, plus (when expanded) each
// NPC (or the "+ add" affordance), each Scroll, each Trail, the find
// affordance, each Rune, each Track, each Lookout, and the write-plans /
// restore affordances. Section headers and paste hints aren't navigable. Used
// to bound link-cursor movement.
// focusLinkCount MUST equal len(m.focusLinks) after focusCodeLines renders —
// they're two views of the same stop list, and any drift breaks Down-nav (see
// TestFocusLinkCountMatchesRender, which locks this invariant). Items, then one
// copy stop per non-empty section header, then the affordances.
func (m *Model) focusLinkCount(q *model.Quest) int {
	trails := len(m.prStack(q.PRs))
	n := len(q.JiraCodes) + trails + len(q.AgentWorkspaces) + len(q.Runes) + len(q.Tracks) + len(q.Lookouts)
	// One focusable copy stop per non-empty section header.
	for _, count := range []int{len(q.AgentWorkspaces), len(q.JiraCodes), trails, len(q.Runes), len(q.Tracks), len(q.Lookouts)} {
		if count > 0 {
			n++
		}
	}
	// Affordances.
	if len(q.AgentWorkspaces) == 0 {
		n++ // the "+ add an NPC" affordance (only when none pinned)
	}
	if trails > 0 {
		n++ // the find affordance (only when there are trails)
	}
	if len(q.Tracks) > 0 {
		n++ // the "write the Lookout's plans" affordance (only with tracks)
	}
	if len(q.DismissedTracks) > 0 {
		n++ // the restore-dismissed affordance
	}
	return n
}

// renderQuestMetaLine renders the integration sub-line for a RowQuestMeta,
// indented to align under the quest's title, and returns the clickable code
// spans (absolute screen columns). Jira and PR groups are kept close together
// (two spaces apart).
func (m *Model) renderQuestMetaLine(row ui.Row, width, xBase int) (string, []codeSpan) {
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
	x := xBase + indent
	var spans []codeSpan
	for i, seg := range segs {
		if i > 0 {
			b.WriteString("  ")
			x += 2
		}
		// Only the code text itself is clickable, not the trailing glyph /
		// counts — but hit-testing the whole segment is close enough and
		// simpler, so the span covers the segment's code+glyph extent. Segments
		// with no URL (the agent spark) aren't clickable here.
		if seg.url != "" {
			spans = append(spans, codeSpan{x0: x, x1: x + seg.width, url: seg.url})
		}
		b.WriteString(seg.text)
		x += seg.width
	}
	return b.String(), spans
}
