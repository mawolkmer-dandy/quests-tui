package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
)

type RowKind int

const (
	RowProject RowKind = iota
	RowQuest
	RowSection
	RowNewProject
	RowNewQuest
	RowSpacer
	RowLabel
)

// Row is one visible line of the outline. Quest rows under a project don't
// need their own project tag; quest rows surfaced in the Vault (which spans
// every project) do, hence ShowProjectTag. Nested marks a campaign (and its
// quests) that render inside the Vault (an archived campaign), so they can
// be indented one level further — visually confirming they're a child of
// the Vault, not another top-level campaign.
type Row struct {
	Kind           RowKind
	ProjectID      string
	QuestID        string
	Section        string // "inbox" | "someday", for RowSection
	Label          string // for RowLabel
	Collapsed      bool
	ShowProjectTag bool
	Nested         bool
}

// Selectable reports whether a row can ever be the cursor target — spacers
// are purely visual, but the "Campaigns" label doubles as a collapse/expand-
// all button (see RowLabel in RenderRow), so it's selectable too.
func (r Row) Selectable() bool {
	switch r.Kind {
	case RowProject, RowQuest, RowSection, RowNewProject, RowNewQuest, RowLabel:
		return true
	}
	return false
}

func findProject(s *store.Store, id string) *model.Project {
	for i := range s.Projects {
		if s.Projects[i].ID == id {
			return &s.Projects[i]
		}
	}
	return nil
}

func findQuest(s *store.Store, id string) *model.Quest {
	for i := range s.Quests {
		if s.Quests[i].ID == id {
			return &s.Quests[i]
		}
	}
	return nil
}

// QuestGlyph is the one icon shown for a quest everywhere (the outline row,
// the detail modal's header): shape encodes progress, color encodes type —
// shared so the two places can never drift apart. A Questboard quest has no
// progress to show yet, so it gets the RPG "quest available" notice mark
// instead of a diamond.
func QuestGlyph(q *model.Quest) (string, lipgloss.Style) {
	style := StyleSide
	if q.Type == model.QuestTypeMain {
		style = StyleMain
	}

	if q.InQuestboard() {
		glyph := GlyphNoticeSide
		if q.Type == model.QuestTypeMain {
			glyph = GlyphNoticeMain
		}
		return glyph, style
	}

	glyph := GlyphQuestOpen
	switch q.Status {
	case model.StatusActive:
		glyph = GlyphQuestActive
	case model.StatusDone:
		glyph = GlyphQuestDone
	}
	return glyph, style
}

// questPriority is a quest's sort tier within its list (lower sorts higher
// up). With no toggles on, every quest is tier 2, so the list keeps the
// manual order you arrange it in. The config toggles carve out tiers:
// important (priority) quests to the top, then main quests, then everyone
// else, with done quests sunk below all of them. It's a flat 4-tier scheme
// on purpose — "priority" is one group regardless of main/side, so within a
// tier ordering stays manual (see SortBucket, moveQuest).
func questPriority(q model.Quest) int {
	switch {
	case DoneToBottom && q.Status == model.StatusDone:
		return 3
	case MovePriorityToTop && q.Important:
		return 0
	case MoveMainToTop && q.Type == model.QuestTypeMain:
		return 1
	default:
		return 2
	}
}

// SortBucket exposes a quest's sort tier so the app can (a) let Shift+↑/↓
// reorder only within a tier and (b) float a quest to the top of its new
// tier when a toggle moves it between tiers.
func SortBucket(q model.Quest) int { return questPriority(q) }

// sortForListing orders quests by questPriority, stable within a bucket.
func sortForListing(quests []model.Quest) []model.Quest {
	out := make([]model.Quest, len(quests))
	copy(out, quests)
	sort.SliceStable(out, func(i, j int) bool {
		return questPriority(out[i]) < questPriority(out[j])
	})
	return out
}

func questsForProject(s *store.Store, projectID string) []model.Quest {
	var out []model.Quest
	for _, q := range s.Quests {
		if q.ProjectID == projectID && !q.Vaulted {
			out = append(out, q)
		}
	}
	return sortForListing(out)
}

func questsForInbox(s *store.Store) []model.Quest {
	var out []model.Quest
	for _, q := range s.Quests {
		if q.ProjectID == "" && !q.Vaulted {
			out = append(out, q)
		}
	}
	return sortForListing(out)
}

func questsForSomeday(s *store.Store) []model.Quest {
	var out []model.Quest
	for _, q := range s.Quests {
		if q.Vaulted {
			out = append(out, q)
		}
	}
	return sortForListing(out)
}

func projectProgress(s *store.Store, projectID string) (done, total int) {
	for _, q := range s.Quests {
		if q.ProjectID != projectID {
			continue
		}
		total++
		if q.Status == model.StatusDone {
			done++
		}
	}
	return done, total
}

// ProjectProgress and QuestsForCampaign are exported so the campaign detail
// modal can show the same progress ring and quest ordering as the outline.
func ProjectProgress(s *store.Store, projectID string) (done, total int) {
	return projectProgress(s, projectID)
}

func QuestsForCampaign(s *store.Store, projectID string) []model.Quest {
	return questsForProject(s, projectID)
}

// QuestsForInbox / QuestsForSomeday expose the Questboard and Vault quest
// lists so their focused pages list the same quests, in the same order, as
// the outline.
func QuestsForInbox(s *store.Store) []model.Quest   { return questsForInbox(s) }
func QuestsForSomeday(s *store.Store) []model.Quest { return questsForSomeday(s) }

func CountInbox(s *store.Store) int   { return len(questsForInbox(s)) }
func CountSomeday(s *store.Store) int { return len(questsForSomeday(s)) }

func CountArchived(s *store.Store) int {
	n := 0
	for _, p := range s.Projects {
		if p.Archived {
			n++
		}
	}
	return n
}

// BuildRows computes the flat list of currently-visible rows: the
// Questboard (Inbox) first, then a "Campaigns" label followed by each
// non-archived campaign (with its quests, unless collapsed) and a
// "+ New Campaign" affordance, then the Vault — parked quests and archived
// campaigns together, since both are simply "not currently active". Rebuilt
// fresh every frame from the store + collapse state — cheap at
// personal-todo-list scale, and avoids ever letting a cached row list drift
// out of sync with a mutation.
func BuildRows(s *store.Store, collapsedProjects, collapsedSections map[string]bool) []Row {
	var rows []Row

	// allowNewQuest is false for archived campaigns nested in the Vault —
	// quests only enter the Vault via Ctrl+V, never created there directly.
	appendProject := func(p model.Project, nested, allowNewQuest bool) {
		collapsed := collapsedProjects[p.ID]
		rows = append(rows, Row{Kind: RowProject, ProjectID: p.ID, Collapsed: collapsed, Nested: nested})
		if collapsed {
			return
		}
		for _, q := range questsForProject(s, p.ID) {
			rows = append(rows, Row{Kind: RowQuest, ProjectID: p.ID, QuestID: q.ID, Nested: nested})
		}
		if allowNewQuest {
			rows = append(rows, Row{Kind: RowNewQuest, ProjectID: p.ID, Nested: nested})
		}
	}

	inboxCollapsed := collapsedSections["inbox"]
	rows = append(rows, Row{Kind: RowSection, Section: "inbox", Collapsed: inboxCollapsed})
	if !inboxCollapsed {
		for _, q := range questsForInbox(s) {
			rows = append(rows, Row{Kind: RowQuest, QuestID: q.ID})
		}
		rows = append(rows, Row{Kind: RowNewQuest})
	}

	rows = append(rows, Row{Kind: RowLabel, Label: "Campaigns", Collapsed: allCampaignsCollapsed(s, collapsedProjects)})
	for _, p := range s.Projects {
		if !p.Archived {
			appendProject(p, false, true)
		}
	}
	rows = append(rows, Row{Kind: RowNewProject})

	vaultCollapsed := collapsedSections["someday"]
	rows = append(rows, Row{Kind: RowSection, Section: "someday", Collapsed: vaultCollapsed})
	if !vaultCollapsed {
		for _, q := range questsForSomeday(s) {
			rows = append(rows, Row{Kind: RowQuest, ProjectID: q.ProjectID, QuestID: q.ID, ShowProjectTag: q.ProjectID != ""})
		}
		for _, p := range s.Projects {
			if p.Archived {
				appendProject(p, true, false)
			}
		}
	}

	return addSpacers(rows)
}

// addSpacers inserts a blank, non-selectable row before each top-level
// group (a label, a campaign, the "+ New Campaign" row, or a section
// header) so groups read as visually distinct blocks instead of one packed
// list.
func addSpacers(rows []Row) []Row {
	out := make([]Row, 0, len(rows)+8)
	for i, r := range rows {
		if i > 0 && (r.Kind == RowProject || r.Kind == RowNewProject || r.Kind == RowSection || r.Kind == RowLabel) {
			out = append(out, Row{Kind: RowSpacer})
		}
		out = append(out, r)
	}
	return out
}

// allCampaignsCollapsed reports whether every non-archived campaign is
// currently collapsed (and there's at least one) — drives the reactive
// "expand all"/"collapse all" hint shown on the "Campaigns" label.
func allCampaignsCollapsed(s *store.Store, collapsedProjects map[string]bool) bool {
	any := false
	for _, p := range s.Projects {
		if p.Archived {
			continue
		}
		any = true
		if !collapsedProjects[p.ID] {
			return false
		}
	}
	return any
}

func caret(collapsed bool) string {
	if collapsed {
		return GlyphCollapsed
	}
	return GlyphExpanded
}

func sectionInfo(s *store.Store, section string) (string, int) {
	switch section {
	case "inbox":
		return "Questboard", CountInbox(s)
	case "someday":
		return "Vault", CountSomeday(s) + CountArchived(s)
	}
	return section, 0
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// RenderRow renders one outline row as a single line. titleView, if
// non-empty, replaces the row's plain title/name text — used to splice in
// the live textinput.View() for whichever row is currently being edited.
// hint, if non-empty, is the pre-rendered action tip placed inline right
// after the row's content (before a campaign's right-aligned progress);
// hintX reports the display column it starts at (-1 when absent) so its
// parts can be made clickable.
func RenderRow(row Row, s *store.Store, titleView string, isCursor bool, width int, hint string) (line string, hintX int) {
	cursorMark := "  "
	if isCursor {
		cursorMark = StyleCursor.Render(GlyphCursor)
	}
	nestIndent := ""
	if row.Nested {
		nestIndent = "  "
	}
	hintX = -1
	withHint := func(content string) string {
		if hint == "" {
			return content
		}
		hintX = lipgloss.Width(content)
		return content + hint
	}

	switch row.Kind {
	case RowProject:
		p := findProject(s, row.ProjectID)
		if p == nil {
			return "", -1
		}
		name := titleView
		if name == "" {
			name = StyleTitle.Render(p.Name)
		}
		done, total := projectProgress(s, p.ID)
		progress := StyleMuted.Render(fmt.Sprintf("%s %d/%d", model.ProgressBucket(done, total), done, total))
		line = withHint(fmt.Sprintf("%s%s%s %s", cursorMark, nestIndent, caret(row.Collapsed), name))
		pad := width - lipgloss.Width(progress) - 1
		if pad < lipgloss.Width(line) {
			return line + " " + progress, hintX
		}
		return padRight(line, pad) + progress, hintX

	case RowQuest:
		q := findQuest(s, row.QuestID)
		if q == nil {
			return "", -1
		}
		glyph, glyphStyle := QuestGlyph(q)
		iconView := glyphStyle.Render(glyph)
		title := titleView
		if title == "" {
			if q.Status == model.StatusDone {
				title = StyleDone.Render(q.Title)
			} else {
				title = StyleTitle.Render(q.Title)
			}
		}
		tag := ""
		if row.ShowProjectTag {
			if p := findProject(s, row.ProjectID); p != nil {
				tag = StyleMuted.Render(" [" + p.Name + "]")
			}
		}
		progress := ""
		if done, total := q.ObjectiveProgress(); total > 0 {
			progress = StyleMuted.Render(fmt.Sprintf(" %d/%d", done, total))
		}
		// The 4-col slot before the glyph holds the priority arrow when the
		// quest is flagged important, else stays blank — either way 4 wide,
		// so glyphs stay column-aligned across the list.
		importIndicator := "    "
		if q.Important {
			importIndicator = "  " + StyleImportant.Render(GlyphImportant) + " "
		}
		return withHint(fmt.Sprintf("%s%s%s%s %s%s%s", cursorMark, nestIndent, importIndicator, iconView, title, tag, progress)), hintX

	case RowSection:
		label, count := sectionInfo(s, row.Section)
		return withHint(StyleSectionHeader.Render(fmt.Sprintf("%s%s %s (%d)", cursorMark, caret(row.Collapsed), label, count))), hintX

	case RowNewProject:
		return cursorMark + StyleMuted.Render("+ New Campaign"), -1

	case RowNewQuest:
		return fmt.Sprintf("%s%s    %s", cursorMark, nestIndent, StyleMuted.Render("+ New Quest")), -1

	case RowLabel:
		return withHint(cursorMark + StyleSectionHeader.Render(row.Label)), hintX

	case RowSpacer:
		return "", -1
	}

	return "", -1
}
