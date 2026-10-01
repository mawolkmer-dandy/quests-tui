package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The room no longer leads with a standalone synced row — the freshness rides on
// each quest header's resync affordance instead.
func TestTrailsRoomNoStandaloneSyncedRow(t *testing.T) {
	m := trailsModel(t)
	for _, r := range m.trailsRows() {
		if r.Kind == ui.RowDayHeader && strings.Contains(r.Label, "synced") {
			t.Errorf("synced label should be inline on the affordance, not a row: %+v", r)
		}
	}
}

// Resync is scoped to the found-from-trails rows/sections — not RowLookout (its
// "r" renames) nor the Scrolls/Lookouts sections.
func TestResyncScope(t *testing.T) {
	for _, k := range []ui.RowKind{ui.RowTrailQuest, ui.RowRuneQuest, ui.RowLookoutQuest, ui.RowTrail, ui.RowRune, ui.RowTrack} {
		if !isTrailResyncRow(k) {
			t.Errorf("%v should be a resync row", k)
		}
	}
	if isTrailResyncRow(ui.RowLookout) {
		t.Error("RowLookout uses r for rename, not resync")
	}
	for _, s := range []string{secTrails, secRunes, secTracks} {
		if !isTrailResyncSection(s) {
			t.Errorf("%q should be a resync section", s)
		}
	}
	if isTrailResyncSection(secLookouts) || isTrailResyncSection(secScrolls) {
		t.Error("lookouts/scrolls are not resync sections")
	}
}

// The room quest header shows resync only for quests that have trails (PRs) to
// resync — a dashboard-only quest has none, so no control.
func TestRoomQuestResyncGating(t *testing.T) {
	ui.Init(true)
	m := &Model{store: &store.Store{Quests: []model.Quest{
		{ID: "withpr", Title: "Has PR", PRs: []model.PRLink{{Code: "#1", Repo: "o/r"}}},
		{ID: "nopr", Title: "No PR", Lookouts: []model.Lookout{{URL: "x"}}},
	}}, lastSyncAt: time.Now()}

	if got := m.roomQuestTitle(ui.Row{Kind: ui.RowRuneQuest, QuestID: "withpr", Label: "Has PR"}, true); !strings.Contains(got, "resync") {
		t.Errorf("a PR-backed header should show resync, got %q", got)
	}
	if got := m.roomQuestTitle(ui.Row{Kind: ui.RowLookoutQuest, QuestID: "nopr", Label: "No PR"}, true); strings.Contains(got, "resync") {
		t.Errorf("a PR-less header should not show resync, got %q", got)
	}
}

// syncedAgo counts seconds, then minutes/hours; zero time reads "not synced yet".
func TestSyncedAgo(t *testing.T) {
	if got := syncedAgo(time.Time{}); got != "not synced yet" {
		t.Errorf("zero time → %q", got)
	}
	if got := syncedAgo(time.Now().Add(-5 * time.Second)); got != "synced 5s ago" {
		t.Errorf("5s → %q", got)
	}
	if got := syncedAgo(time.Now().Add(-3 * time.Minute)); got != "synced 3m ago" {
		t.Errorf("3m → %q", got)
	}
}

// The resync affordance is a button when idle and a spinner mid-resync, scoped
// to the quest that's actually resyncing.
func TestResyncAffordance(t *testing.T) {
	ui.Init(true)
	m := &Model{lastSyncAt: time.Now().Add(-5 * time.Second)}
	if got := m.resyncAffordance("q1"); strings.Contains(got, "resyncing") || !strings.Contains(got, "resync") {
		t.Errorf("idle → should be the resync button, got %q", got)
	}
	// The freshness rides inline on the idle button.
	if got := m.resyncAffordance("q1"); !strings.Contains(got, "synced 5s ago") {
		t.Errorf("idle affordance should carry the synced-ago label, got %q", got)
	}
	m.findingQuestID = "q1"
	if got := m.resyncAffordance("q1"); !strings.Contains(got, "resyncing") {
		t.Errorf("mid-resync → should spin, got %q", got)
	}
	if got := m.resyncAffordance("q2"); strings.Contains(got, "resyncing") {
		t.Errorf("another quest → should not spin, got %q", got)
	}
}

func trailsModel(t *testing.T) *Model {
	ui.Init(true)
	m := &Model{
		store: &store.Store{
			Quests: []model.Quest{
				{ID: "alpha", Title: "Alpha", PRs: []model.PRLink{{Code: "#1", Repo: "o/r"}}},
				{ID: "bravo", Title: "Bravo", PRs: []model.PRLink{{Code: "#2", Repo: "o/r"}}},
				{ID: "charlie", Title: "Charlie", PRs: []model.PRLink{{Code: "#3", Repo: "o/r"}}}, // merged → excluded
				{ID: "delta", Title: "Delta", PRs: []model.PRLink{{Code: "#5", Repo: "o/r"}, {Code: "#4", Repo: "o/r"}}},
				{ID: "vaulted", Title: "Vaulted", Vaulted: true, PRs: []model.PRLink{{Code: "#9", Repo: "o/r"}}}, // excluded
			},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}
	m.prStatus = map[string]PRStatus{
		"#1": {Code: "#1", Status: "success", CommentsResolved: 1, CommentsTotal: 1, Approvals: 1, ReviewersTotal: 2, HeadRef: "a", BaseRef: "main"},
		"#2": {Code: "#2", Status: "error", HeadRef: "b", BaseRef: "main"}, // failing → attention
		"#3": {Code: "#3", Status: "merged", HeadRef: "c", BaseRef: "main"},
		"#4": {Code: "#4", Status: "success", HeadRef: "d1", BaseRef: "main", Draft: true}, // stack bottom
		"#5": {Code: "#5", Status: "running", HeadRef: "d2", BaseRef: "d1"},                // stack top (child of #4)
		"#9": {Code: "#9", Status: "success", HeadRef: "z", BaseRef: "main"},
	}
	return m
}

// trailsCount counts only open PRs on un-vaulted quests (merged/closed and
// vaulted quests drop off).
func TestTrailsCount(t *testing.T) {
	m := trailsModel(t)
	if got := m.trailsCount(); got != 4 { // #1, #2, #4, #5 — not #3 (merged) or #9 (vaulted)
		t.Fatalf("trailsCount = %d, want 4", got)
	}
}

func TestPROpen(t *testing.T) {
	m := trailsModel(t)
	if !m.prOpen("#1") || m.prOpen("#3") {
		t.Fatal("open PRs are open; merged PRs are not")
	}
	if !m.prOpen("#unsynced") {
		t.Fatal("a PR with no synced status counts as open until proven settled")
	}
}

// trailsRows groups open PRs by quest, floats attention-needed quests to the top,
// excludes merged/vaulted, and orders a quest's PRs into their stack with tree
// connectors.
func TestTrailsRows(t *testing.T) {
	m := trailsModel(t)
	rows := m.trailsRows()

	// Quest-header order: Bravo (failing → attention) first, then Alpha, Delta by title.
	var headers []string
	for _, r := range rows {
		if r.Kind == ui.RowTrailQuest {
			headers = append(headers, r.Label)
		}
	}
	want := []string{"Bravo", "Alpha", "Delta"}
	if strings.Join(headers, ",") != strings.Join(want, ",") {
		t.Fatalf("group order = %v, want %v", headers, want)
	}

	// Charlie (only a merged PR) and the vaulted quest never appear.
	for _, r := range rows {
		if r.Kind == ui.RowTrailQuest && (r.Label == "Charlie" || r.Label == "Vaulted") {
			t.Fatalf("%s should not appear — no open PRs / vaulted", r.Label)
		}
	}

	// Delta's stack renders top-first with a trunk line: #5 (child, top) then #4
	// (parent, targets main), both with "├", then a "└ main" trunk row.
	var deltaRows []ui.Row
	inDelta := false
	for _, r := range rows {
		if r.Kind == ui.RowTrailQuest {
			inDelta = r.Label == "Delta"
			continue
		}
		if inDelta && (r.Kind == ui.RowTrail || r.Kind == ui.RowDayHeader) {
			deltaRows = append(deltaRows, r)
		}
		if inDelta && r.Kind == ui.RowSpacer {
			inDelta = false
		}
	}
	if len(deltaRows) != 3 {
		t.Fatalf("Delta should have 2 PR rows + a main trunk row, got %d: %+v", len(deltaRows), deltaRows)
	}
	if deltaRows[0].Code != "#5" || deltaRows[0].Label != ui.GlyphStackBranchMid {
		t.Errorf("top of stack should be #5 with ├, got %s / %q", deltaRows[0].Code, deltaRows[0].Label)
	}
	if deltaRows[1].Code != "#4" || deltaRows[1].Label != ui.GlyphStackBranchMid {
		t.Errorf("parent should be #4 with ├, got %s / %q", deltaRows[1].Code, deltaRows[1].Label)
	}
	if deltaRows[2].Kind != ui.RowDayHeader || !strings.Contains(deltaRows[2].Label, "main") {
		t.Errorf("stack should close with a 'main' trunk row, got %+v", deltaRows[2])
	}

	// A lone PR (Alpha's #1) gets no connector.
	for _, r := range rows {
		if r.Kind == ui.RowTrail && r.Code == "#1" && r.Label != "" {
			t.Errorf("a lone PR should have no stack connector, got %q", r.Label)
		}
	}
}

// trailRowContent shows the CI word, a draft marker for drafts, and the comment
// count.
func TestTrailRowContent(t *testing.T) {
	m := trailsModel(t)
	if got := m.trailRowContent("#2"); !strings.Contains(got, "#2") || !strings.Contains(got, "failing") {
		t.Errorf("failing PR content should name it and say failing: %q", got)
	}
	if got := m.trailRowContent("#4"); !strings.Contains(got, "draft") {
		t.Errorf("a draft PR should be marked draft: %q", got)
	}
	// Comments and approvals show as icon-led counts (no "comments"/"approved"
	// words): 󰆂 1/1 for resolved threads, 󰀈 1/2 for approvals.
	got := m.trailRowContent("#1")
	if !strings.Contains(got, ui.GlyphPRComment+" 1/1") {
		t.Errorf("content should show the comment count with its icon: %q", got)
	}
	if !strings.Contains(got, ui.GlyphPRApproval+" 1/2") {
		t.Errorf("content should show the approval count with its icon: %q", got)
	}
	// A PR with no reviewers (#2) hides the approvals badge entirely.
	if strings.Contains(m.trailRowContent("#2"), ui.GlyphPRApproval) {
		t.Error("a PR with no reviewers should not show an approvals badge")
	}
}

// stackDisplay orders a stack top-first (head → parent) with "├" connectors and
// closes it with a "└ main" trunk line; a lone PR gets neither.
func TestStackDisplay(t *testing.T) {
	nodes := []prStackNode{
		{link: model.PRLink{Code: "#1"}, depth: 0, stacked: true}, // root (targets main)
		{link: model.PRLink{Code: "#2"}, depth: 1, stacked: true},
		{link: model.PRLink{Code: "#3"}, depth: 2, stacked: true}, // head (top)
	}
	got := stackDisplay(nodes)
	if len(got) != 4 {
		t.Fatalf("want 3 PRs + main = 4 items, got %d", len(got))
	}
	for i, code := range []string{"#3", "#2", "#1"} { // top-first
		if got[i].isMain || got[i].node.link.Code != code || got[i].marker != ui.GlyphStackBranchMid {
			t.Errorf("item %d = %+v, want %s with ├", i, got[i], code)
		}
	}
	if !got[3].isMain || got[3].marker != ui.GlyphStackBranchEnd {
		t.Errorf("last item should be the main trunk with └, got %+v", got[3])
	}

	lone := stackDisplay([]prStackNode{{link: model.PRLink{Code: "#9"}, stacked: false}})
	if len(lone) != 1 || lone[0].isMain || lone[0].marker != "" {
		t.Fatalf("a lone PR should be one item with no connector/trunk, got %+v", lone)
	}
}

// The room stays visible once you have any linked PR (so it doesn't vanish when
// your last open PR merges); with nothing open it shows a calm empty state, not
// PR rows.
func TestTrailsPersistsWhenAllMerged(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store: &store.Store{
			Quests: []model.Quest{{ID: "q", Title: "Shipped", PRs: []model.PRLink{{Code: "#1", Repo: "o/r"}}}},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}
	m.prStatus = map[string]PRStatus{"#1": {Code: "#1", Status: "merged"}}

	if !m.hasAnyTrails() {
		t.Fatal("a quest with a (merged) PR keeps the Trails room visible")
	}
	if m.trailsCount() != 0 {
		t.Fatalf("no OPEN PRs → badge 0, got %d", m.trailsCount())
	}
	rows := m.trailsRows()
	sawEmptyState := false
	for _, r := range rows {
		if r.Kind == ui.RowTrail {
			t.Fatal("all-merged should show no PR rows")
		}
		if r.Kind == ui.RowDayHeader && strings.Contains(r.Label, "No open trails") {
			sawEmptyState = true
		}
	}
	if !sawEmptyState {
		t.Fatalf("all-merged should show the calm empty-state line, got %+v", rows)
	}
}

// The room quest headers are not collapsible — every open PR shows at a glance.
func TestTrailGroupNotCollapsible(t *testing.T) {
	m := trailsModel(t)
	for _, r := range m.trailsRows() {
		if r.Kind == ui.RowTrailQuest && r.Collapsed {
			t.Error("a Trails-room quest header should never be collapsed")
		}
	}
	// delta's two stacked PRs both render (no group ever hides them).
	deltaPRs := 0
	for _, r := range m.trailsRows() {
		if r.Kind == ui.RowTrail && r.QuestID == "delta" {
			deltaPRs++
		}
	}
	if deltaPRs != 2 {
		t.Errorf("delta's stacked PRs should both show, got %d", deltaPRs)
	}
}
