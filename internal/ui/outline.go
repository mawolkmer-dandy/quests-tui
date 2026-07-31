package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

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
	// RowQuestMeta is the non-selectable integration sub-line rendered just
	// below a quest row that has a Jira/PR link (see internal/app rendering).
	// It's deliberately absent from Selectable() so cursor nav and the mouse
	// skip it, keeping one selectable row per screen line.
	RowQuestMeta
	// RowRune is one watched LaunchDarkly flag in the Tavern's Runes section;
	// RowNewRune is the "+ watch a rune" affordance below them.
	RowRune
	RowNewRune
	// RowDayHeader is a non-selectable date divider in the Vault's timeline
	// (Label holds the day). RowVaultCampaign is a retired campaign shown as a
	// single muted inline row at the top of the Vault. RowRuneQuest is a
	// collapsible quest header in the Runes section (its runes are its children).
	RowDayHeader
	RowVaultCampaign
	RowRuneQuest
	// RowWildsObjective is a single pending objective listed beneath its quest
	// in the Wilds view — selectable (up/down step onto it) and indented under
	// the quest. BodyLineID identifies which body line it maps to; Ctrl+D marks
	// that line done, which drops the row from the list.
	RowWildsObjective
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
	RuneKey        string // for RowRune: the LaunchDarkly flag key
	BodyLineID     string // for RowWildsObjective: the quest body line it maps to
}

// Selectable reports whether a row can ever be the cursor target — spacers
// are purely visual, but the "Campaigns" label doubles as a collapse/expand-
// all button (see RowLabel in RenderRow), so it's selectable too.
func (r Row) Selectable() bool {
	switch r.Kind {
	case RowProject, RowQuest, RowSection, RowNewProject, RowNewQuest, RowLabel, RowRune, RowNewRune, RowVaultCampaign, RowRuneQuest, RowWildsObjective:
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
// ObjectiveCheckbox is the muted checkbox glyph for an objective — hollow when
// pending, filled when done. The single source both the outline (Wilds) and
// the detail-page body render objectives through, so they can't drift apart.
func ObjectiveCheckbox(done bool) string {
	g := GlyphQuestOpen
	if done {
		g = GlyphQuestDone
	}
	return StyleMuted.Render(g)
}

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
// high then medium priority quests to the top, then main quests, then
// everyone else, then low-priority quests, with done quests sunk below all of
// them. Within a tier ordering stays manual (see SortBucket, moveQuest).
func questPriority(q model.Quest) int {
	switch {
	case DoneToBottom && q.Status == model.StatusDone:
		return 5
	case LowPriorityToBottom && q.Priority == model.PriorityLow:
		return 4
	case MovePriorityToTop && q.Priority == model.PriorityHigh:
		return 0
	case MovePriorityToTop && q.Priority == model.PriorityMedium:
		return 1
	case MoveMainToTop && q.Type == model.QuestTypeMain:
		return 2
	default:
		return 3
	}
}

// priorityIndicator renders the 2-col slot left of a quest's glyph for its
// priority level (arrow + space), blank when none — kept a fixed width so
// glyphs stay aligned.
func priorityIndicator(p model.Priority) string {
	switch p {
	case model.PriorityMedium:
		return StylePriorityMedium.Render(GlyphImportant) + " "
	case model.PriorityHigh:
		return StyleImportant.Render(GlyphImportant) + " "
	case model.PriorityLow:
		return StyleMuted.Render(GlyphPriorityLow) + " "
	default:
		return "  "
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
	rows = append(rows, inboxRows(s, collapsedSections)...)
	rows = append(rows, campaignRows(s, collapsedProjects)...)
	rows = append(rows, runesRows(s, collapsedProjects, collapsedSections)...)
	rows = append(rows, vaultRows(s, collapsedProjects, collapsedSections)...)
	return addSpacers(rows)
}

// SectionContent is a section's rows WITHOUT its header — the exact content the
// Tavern box shows for that section, so a focused section view renders
// identically (day-grouped Vault, spaced campaigns, grouped Runes), just with
// more vertical room. collapsedProjects drives per-quest rune groups and
// archived-campaign nesting; the section itself is always expanded here.
func SectionContent(s *store.Store, section string, collapsedProjects map[string]bool) []Row {
	var full []Row
	switch section {
	case "inbox":
		full = inboxRows(s, nil)
	case "runes":
		full = runesRows(s, collapsedProjects, nil)
	case "someday":
		full = vaultRows(s, collapsedProjects, nil)
	case "campaigns":
		full = BuildCampaignColumn(s, collapsedProjects)
	}
	if len(full) > 0 {
		return full[1:] // drop the RowSection / RowLabel header
	}
	return nil
}

// BuildCampaignColumn is the two-column Tavern's campaigns column (the right
// side): the Campaigns label followed by every campaign and its quests. Each
// campaign is separated by a spacer for breathing room inside its box.
func BuildCampaignColumn(s *store.Store, collapsedProjects map[string]bool) []Row {
	return addSpacers(campaignRows(s, collapsedProjects))
}

// BuildRailColumn is the two-column Tavern's left rail: Questboard, then Runes,
// then the Vault — three sections concatenated (each begins with its
// RowSection header). The renderer splits on those headers into separate boxes;
// no spacers, so it can lay them out and pin the Vault itself.
func BuildRailColumn(s *store.Store, collapsedProjects, collapsedSections map[string]bool) []Row {
	var rows []Row
	rows = append(rows, inboxRows(s, collapsedSections)...)
	rows = append(rows, runesRows(s, collapsedProjects, collapsedSections)...)
	rows = append(rows, vaultRows(s, collapsedProjects, collapsedSections)...)
	return rows
}

// appendProject appends a campaign header and, unless collapsed, its quests
// (and a "+ New Quest" affordance when allowNewQuest). allowNewQuest is false
// for archived campaigns nested in the Vault — quests only enter the Vault via
// Ctrl+V, never created there directly.
func appendProject(s *store.Store, rows []Row, p model.Project, collapsedProjects map[string]bool, nested, allowNewQuest bool) []Row {
	collapsed := collapsedProjects[p.ID]
	rows = append(rows, Row{Kind: RowProject, ProjectID: p.ID, Collapsed: collapsed, Nested: nested})
	if collapsed {
		return rows
	}
	for _, q := range questsForProject(s, p.ID) {
		rows = append(rows, Row{Kind: RowQuest, ProjectID: p.ID, QuestID: q.ID, Nested: nested})
	}
	if allowNewQuest {
		rows = append(rows, Row{Kind: RowNewQuest, ProjectID: p.ID, Nested: nested})
	}
	return rows
}

func inboxRows(s *store.Store, collapsedSections map[string]bool) []Row {
	collapsed := collapsedSections["inbox"]
	rows := []Row{{Kind: RowSection, Section: "inbox", Collapsed: collapsed}}
	if collapsed {
		return rows
	}
	for _, q := range questsForInbox(s) {
		rows = append(rows, Row{Kind: RowQuest, QuestID: q.ID})
	}
	return append(rows, Row{Kind: RowNewQuest})
}

func campaignRows(s *store.Store, collapsedProjects map[string]bool) []Row {
	rows := []Row{{Kind: RowLabel, Label: "Campaigns", Collapsed: allCampaignsCollapsed(s, collapsedProjects)}}
	for _, p := range s.Projects {
		if !p.Archived {
			rows = appendProject(s, rows, p, collapsedProjects, false, true)
		}
	}
	return append(rows, Row{Kind: RowNewProject})
}

// runesRows draws the Runes section from every un-vaulted quest's attached
// flags (no manual watch list) — grouped under a collapsible quest header.
// Vaulted quests drop out (their flags aren't worth watching anymore). Each
// quest group's collapse state is keyed by quest ID in collapsedProjects
// (quest IDs never collide with project IDs).
func runesRows(s *store.Store, collapsedProjects, collapsedSections map[string]bool) []Row {
	collapsed := collapsedSections["runes"]
	rows := []Row{{Kind: RowSection, Section: "runes", Collapsed: collapsed}}
	if collapsed {
		return rows
	}
	first := true
	for i := range s.Quests {
		q := &s.Quests[i]
		if len(q.Runes) == 0 || questVaulted(s, q) {
			continue
		}
		if !first {
			rows = append(rows, Row{Kind: RowSpacer})
		}
		first = false
		qCollapsed := collapsedProjects[q.ID]
		rows = append(rows, Row{Kind: RowRuneQuest, Label: q.Title, QuestID: q.ID, Collapsed: qCollapsed})
		if qCollapsed {
			continue
		}
		for _, key := range q.Runes {
			rows = append(rows, Row{Kind: RowRune, RuneKey: key, QuestID: q.ID})
		}
	}
	return rows
}

// questVaulted reports whether a quest is parked in the Vault directly or via
// an archived campaign.
func questVaulted(s *store.Store, q *model.Quest) bool {
	if q.Vaulted {
		return true
	}
	if q.ProjectID != "" {
		if p := findProject(s, q.ProjectID); p != nil && p.Archived {
			return true
		}
	}
	return false
}

// CountRunes is the number of rune rows shown in the Tavern (attached runes
// across un-vaulted quests).
func CountRunes(s *store.Store) int {
	n := 0
	for i := range s.Quests {
		if q := &s.Quests[i]; !questVaulted(s, q) {
			n += len(q.Runes)
		}
	}
	return n
}

func vaultRows(s *store.Store, collapsedProjects, collapsedSections map[string]bool) []Row {
	collapsed := collapsedSections["someday"]
	rows := []Row{{Kind: RowSection, Section: "someday", Collapsed: collapsed}}
	if collapsed {
		return rows
	}
	// Parked quests AND retired campaigns share one timeline, grouped by the day
	// they entered the Vault (newest first), each day a divider. A retired
	// campaign sits inline like a quest on the day it was archived.
	var entries []vaultEntry
	for _, q := range questsForSomeday(s) {
		entries = append(entries, vaultEntry{when: q.VaultedAt, row: Row{Kind: RowQuest, ProjectID: q.ProjectID, QuestID: q.ID, ShowProjectTag: q.ProjectID != ""}})
	}
	for i := range s.Projects {
		if p := &s.Projects[i]; p.Archived {
			entries = append(entries, vaultEntry{when: p.ArchivedAt, row: Row{Kind: RowVaultCampaign, ProjectID: p.ID}})
		}
	}
	for i, g := range groupVaultEntries(entries) {
		if i > 0 {
			rows = append(rows, Row{Kind: RowSpacer})
		}
		rows = append(rows, Row{Kind: RowDayHeader, Label: g.label})
		rows = append(rows, g.rows...)
	}
	return rows
}

// vaultEntry is one thing in the Vault (a parked quest or a retired campaign)
// and when it got there (nil = before the app tracked it → "Earlier").
type vaultEntry struct {
	when *time.Time
	row  Row
}

// vaultGroup is one day's worth of Vault entries (already rendered to rows).
type vaultGroup struct {
	label string
	rows  []Row
}

// groupVaultEntries buckets entries by the day they entered the Vault, newest
// day first (Today / Yesterday / date). Entries with no timestamp collect in a
// single "Earlier" bucket at the bottom rather than a misleading fallback date.
func groupVaultEntries(entries []vaultEntry) []vaultGroup {
	var dated, undated []vaultEntry
	for _, e := range entries {
		if e.when != nil {
			dated = append(dated, e)
		} else {
			undated = append(undated, e)
		}
	}
	sort.SliceStable(dated, func(i, j int) bool {
		return dated[i].when.After(*dated[j].when)
	})

	var groups []vaultGroup
	byKey := map[string]int{}
	now := time.Now()
	for _, e := range dated {
		key := e.when.Format("2006-01-02")
		idx, ok := byKey[key]
		if !ok {
			idx = len(groups)
			byKey[key] = idx
			groups = append(groups, vaultGroup{label: relativeDay(*e.when, now)})
		}
		groups[idx].rows = append(groups[idx].rows, e.row)
	}
	if len(undated) > 0 {
		g := vaultGroup{label: "Earlier"}
		for _, e := range undated {
			g.rows = append(g.rows, e.row)
		}
		groups = append(groups, g)
	}
	return groups
}

// relativeDay labels t as Today / Yesterday, or a "Mon, Jan 2" date otherwise.
func relativeDay(t, now time.Time) string {
	d := daysBetween(t, now)
	switch d {
	case 0:
		return "Today"
	case 1:
		return "Yesterday"
	}
	if t.Year() == now.Year() {
		return t.Format("Mon, Jan 2")
	}
	return t.Format("Mon, Jan 2 2006")
}

func daysBetween(t, now time.Time) int {
	ty, tm, td := t.Date()
	ny, nm, nd := now.Date()
	a := time.Date(ty, tm, td, 0, 0, 0, 0, time.Local)
	b := time.Date(ny, nm, nd, 0, 0, 0, 0, time.Local)
	return int(b.Sub(a).Hours() / 24)
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
	case "runes":
		return "Runes", CountRunes(s)
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
			name = StyleName.Render(p.Name)
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
				title = StyleName.Render(q.Title)
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
		// The 4-col slot before the glyph holds the priority arrow (up for
		// medium/high, a muted down-arrow for low), else stays blank — either
		// way 4 wide, so glyphs stay column-aligned across the list.
		return withHint(fmt.Sprintf("%s%s%s%s %s%s%s", cursorMark, nestIndent, priorityIndicator(q.Priority), iconView, title, tag, progress)), hintX

	case RowSection:
		label, count := sectionInfo(s, row.Section)
		// The Vault reads like an old strongbox: an aged/rusted label with a
		// heavy-door block ornament, rather than a plain live section header.
		if row.Section == "someday" {
			framed := StyleVaultFrame.Render(fmt.Sprintf("▓ %s (%d)", label, count))
			return withHint(fmt.Sprintf("%s%s %s", cursorMark, caret(row.Collapsed), framed)), hintX
		}
		return withHint(StyleSectionHeader.Render(fmt.Sprintf("%s%s %s (%d)", cursorMark, caret(row.Collapsed), label, count))), hintX

	case RowNewProject:
		return cursorMark + StyleMuted.Render("+ New Campaign"), -1

	case RowNewQuest:
		return fmt.Sprintf("%s%s    %s", cursorMark, nestIndent, StyleMuted.Render("+ New Quest")), -1

	case RowWildsObjective:
		q := findQuest(s, row.QuestID)
		if q == nil {
			return "", -1
		}
		display, extraIndent, found := "", 0, false
		for _, l := range q.Body {
			if l.ID == row.BodyLineID {
				_, display = model.ClassifyBodyLine(l.Text)
				extraIndent = 2 * l.Indent
				found = true
				break
			}
		}
		if !found {
			return "", -1
		}
		// Indented under the quest (past its priority slot + status glyph) so it
		// reads as a child; the checkbox sits under the quest title.
		indent := strings.Repeat(" ", 6+extraIndent)
		check := ObjectiveCheckbox(false) // the Wilds only lists pending objectives
		return withHint(fmt.Sprintf("%s%s%s %s", cursorMark, indent, check, StyleMuted.Render(display))), hintX

	case RowRune:
		// titleView is the app-rendered "glyph key  state" content (the live
		// rollout state lives in the app, not here) — indented under its
		// quest header.
		return withHint(fmt.Sprintf("%s  %s", cursorMark, titleView)), hintX

	case RowNewRune:
		return fmt.Sprintf("%s  %s", cursorMark, StyleMuted.Render("⚹ watch a rune")), -1

	case RowRuneQuest:
		// A collapsible quest header in the Runes section — white name (bold when
		// selected), like a campaign but for the flag groups.
		name := row.Label
		if isCursor {
			name = StyleTitle.Render(name)
		} else {
			name = StyleName.Render(name)
		}
		return withHint(fmt.Sprintf("%s%s %s", cursorMark, caret(row.Collapsed), name)), hintX

	case RowDayHeader:
		return StyleMuted.Render(row.Label), -1

	case RowVaultCampaign:
		p := findProject(s, row.ProjectID)
		if p == nil {
			return "", -1
		}
		name := titleView
		if name == "" {
			name = StyleMuted.Render(p.Name)
		}
		return withHint(fmt.Sprintf("%s%s %s", cursorMark, StyleMuted.Render(GlyphArchived), name)), hintX

	case RowLabel:
		banner := StyleOrnament.Render(GlyphFlourishL) + " " +
			StyleSectionHeader.Render(row.Label) + " " +
			StyleOrnament.Render(GlyphFlourishR)
		return withHint(cursorMark + banner), hintX

	case RowSpacer:
		return "", -1
	}

	return "", -1
}
