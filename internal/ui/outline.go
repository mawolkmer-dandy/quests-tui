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
	// RowRune is one watched LaunchDarkly flag in the Tavern's Runes section.
	RowRune
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
	// RowLookout is one usage dashboard in the Tavern's Lookouts section;
	// RowLookoutQuest is the collapsible quest header grouping that quest's
	// lookouts (and, in the focused page, its tracks). RowTrack is one found
	// tracking event listed under that header.
	RowLookout
	RowLookoutQuest
	RowTrack
	// RowVaultHeader is a collapsible "Vaulted (N)" sub-header shown at the
	// bottom of a focused section page (Runes / Lookouts / Campaigns), grouping
	// the same content for parked/archived quests. Section names the parent
	// section; its expanded state lives under a synthetic collapsedProjects key.
	RowVaultHeader
	// RowBanner is a Banner (Area) header in the campaigns hall; its campaigns
	// nest beneath it, collapsible (keyed by banner ID in collapsedProjects,
	// which never collides with project/quest IDs). RowNewBanner is the
	// "+ New Banner" affordance.
	RowBanner
	RowNewBanner
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
	LookoutURL     string // for RowLookout: the dashboard URL
	TrackEvent     string // for RowTrack: the tracking-event name
	BannerID       string // for RowBanner; also on a RowNewProject to create the campaign inside that banner
	// Bare drops the collapse chevron on a RowBanner/RowProject that isn't
	// collapsible in context — a banner title, or a campaign shown as a
	// drill-in row (Enter opens it, it doesn't expand in place).
	Bare bool
	// Dim mutes a quest/campaign row that's in a "Later" (inactive) group, so
	// active work reads brighter than the backlog beside it.
	Dim bool
	// Header marks the campaign row that titles its own pane — rendered as a
	// page title (bold, UPPERCASED, campaign-accent) a tier below the banner
	// title, so it doesn't read like just another body row.
	Header bool
}

// Selectable reports whether a row can ever be the cursor target — spacers
// are purely visual, but the "Campaigns" label doubles as a collapse/expand-
// all button (see RowLabel in RenderRow), so it's selectable too.
func (r Row) Selectable() bool {
	switch r.Kind {
	case RowProject, RowQuest, RowSection, RowNewProject, RowNewQuest, RowLabel, RowRune, RowVaultCampaign, RowRuneQuest, RowWildsObjective, RowLookout, RowLookoutQuest, RowTrack, RowVaultHeader, RowBanner, RowNewBanner:
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
		if q.InQuestboard() { // no campaign AND no banner — a truly unfiled notice
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
		if p.Archived { // only vaulted campaigns live in the Vault
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
	rows = append(rows, lookoutsRows(s, collapsedProjects, collapsedSections)...)
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
		full = runesRowsOpt(s, collapsedProjects, nil, true)
	case "lookouts":
		full = lookoutsRowsOpt(s, collapsedProjects, nil, true)
	case "someday":
		full = vaultRows(s, collapsedProjects, nil)
	case "campaigns":
		full = appendVaultedGroups(s, BuildCampaignColumn(s, collapsedProjects), collapsedProjects, "campaigns")
	}
	if len(full) == 0 {
		return nil
	}
	rows := full[1:] // drop the RowSection / RowLabel header
	// The campaigns list puts a spacer after its "Campaigns" label; with the
	// label dropped that spacer becomes a blank line above the first campaign,
	// which no other section has. Drop it so every room's first row sits at the
	// same top.
	if len(rows) > 0 && rows[0].Kind == RowSpacer {
		rows = rows[1:]
	}
	return rows
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
	rows = append(rows, lookoutsRows(s, collapsedProjects, collapsedSections)...)
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
	// A campaign shows in the hall unless it's been vaulted (archived). Completed
	// campaigns STAY here (done-styled, like a done quest) until you send them to
	// the Vault yourself with Ctrl+V — nothing auto-moves.
	live := func(p model.Project) bool { return !p.Archived }
	emit := func(bannerID string, nested bool) {
		for _, p := range s.Projects {
			if live(p) && p.BannerID == bannerID {
				rows = appendProject(s, rows, p, collapsedProjects, nested, true)
			}
		}
	}
	// Banners, each grouping its campaigns (indented beneath it); a collapsed
	// banner hides them.
	for _, b := range s.Banners {
		collapsed := collapsedProjects[b.ID]
		rows = append(rows, Row{Kind: RowBanner, BannerID: b.ID, Label: b.Name, Collapsed: collapsed})
		if collapsed {
			continue
		}
		// Loose quests living directly under the banner (ongoing area work) come
		// first, then a "+ New Quest" for them, then the banner's campaigns.
		for _, q := range looseQuestsUnderBanner(s, b.ID) {
			rows = append(rows, Row{Kind: RowQuest, QuestID: q.ID, Nested: true})
		}
		rows = append(rows, Row{Kind: RowNewQuest, BannerID: b.ID, Nested: true})
		emit(b.ID, true)
		rows = append(rows, Row{Kind: RowNewProject, BannerID: b.ID, Nested: true}) // + New Campaign here
	}
	// Ungrouped campaigns (no banner) fall at the end, above the global "+ New".
	emit("", false)
	rows = append(rows, Row{Kind: RowNewBanner})
	return append(rows, Row{Kind: RowNewProject})
}

// findBanner returns the banner with the given ID, or nil.
func findBanner(s *store.Store, id string) *model.Banner {
	for i := range s.Banners {
		if s.Banners[i].ID == id {
			return &s.Banners[i]
		}
	}
	return nil
}

// looseQuestsUnderBanner is the quests that live directly under a banner — no
// campaign (empty ProjectID), not parked — in store order.
func looseQuestsUnderBanner(s *store.Store, bannerID string) []model.Quest {
	var out []model.Quest
	for _, q := range s.Quests {
		if q.ProjectID == "" && q.BannerID == bannerID && !q.Vaulted {
			out = append(out, q)
		}
	}
	return out
}

// BannerCampaignCount exposes bannerCampaignCount for the Tavern hall.
func BannerCampaignCount(s *store.Store, bannerID string) int {
	return bannerCampaignCount(s, bannerID)
}

// bannerCampaignCount is how many live (un-vaulted) campaigns fly under a banner.
func bannerCampaignCount(s *store.Store, bannerID string) int {
	n := 0
	for i := range s.Projects {
		if p := s.Projects[i]; !p.Archived && p.BannerID == bannerID {
			n++
		}
	}
	return n
}

// runesRows draws the Runes section from every un-vaulted quest's attached
// flags (no manual watch list) — grouped under a collapsible quest header.
// Vaulted quests drop out (their flags aren't worth watching anymore). Each
// quest group's collapse state is keyed by quest ID in collapsedProjects
// (quest IDs never collide with project IDs).
func runesRows(s *store.Store, collapsedProjects, collapsedSections map[string]bool) []Row {
	return runesRowsOpt(s, collapsedProjects, collapsedSections, false)
}

func runesRowsOpt(s *store.Store, collapsedProjects, collapsedSections map[string]bool, includeVaulted bool) []Row {
	collapsed := collapsedSections["runes"]
	rows := []Row{{Kind: RowSection, Section: "runes", Collapsed: collapsed}}
	if collapsed {
		return rows
	}
	rows = appendRuneGroups(s, rows, collapsedProjects, false)
	if includeVaulted {
		rows = appendVaultedGroups(s, rows, collapsedProjects, "runes")
	}
	return rows
}

// appendRuneGroups appends one collapsible group per quest (matching vaulted)
// that has runes: the quest header, then its rune rows.
func appendRuneGroups(s *store.Store, rows []Row, collapsedProjects map[string]bool, vaulted bool) []Row {
	first := true
	for i := range s.Quests {
		q := &s.Quests[i]
		if len(q.Runes) == 0 || questVaulted(s, q) != vaulted {
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

// lookoutsRows draws the Lookouts section from every un-vaulted quest with
// analytics — its Tracks (found events) and Lookouts (dashboards) — grouped
// under a collapsible quest header. When includeVaulted is set (the focused
// page), a collapsed "Vaulted" group of the same, for parked quests, follows.
func lookoutsRows(s *store.Store, collapsedProjects, collapsedSections map[string]bool) []Row {
	return lookoutsRowsOpt(s, collapsedProjects, collapsedSections, false)
}

func lookoutsRowsOpt(s *store.Store, collapsedProjects, collapsedSections map[string]bool, includeVaulted bool) []Row {
	collapsed := collapsedSections["lookouts"]
	rows := []Row{{Kind: RowSection, Section: "lookouts", Collapsed: collapsed}}
	if collapsed {
		return rows
	}
	// The compact Tavern rail lists only actual dashboards (Lookouts); Tracks
	// surface only when the Lookouts page is expanded (includeVaulted == focused).
	includeTracks := includeVaulted
	rows = appendAnalyticsGroups(s, rows, collapsedProjects, false, includeTracks)
	if includeVaulted {
		rows = appendVaultedGroups(s, rows, collapsedProjects, "lookouts")
	}
	return rows
}

// appendAnalyticsGroups appends one collapsible group per quest (matching
// vaulted) that has Lookouts (and, when includeTracks, Tracks too): the quest
// header, then its Tracks, then its Lookouts. When includeTracks is false, a
// quest needs at least one Lookout to appear at all — track-only quests are
// hidden so the rail stays minimal.
func appendAnalyticsGroups(s *store.Store, rows []Row, collapsedProjects map[string]bool, vaulted, includeTracks bool) []Row {
	first := true
	for i := range s.Quests {
		q := &s.Quests[i]
		relevant := len(q.Lookouts) > 0 || (includeTracks && len(q.Tracks) > 0)
		if questVaulted(s, q) != vaulted || !relevant {
			continue
		}
		if !first {
			rows = append(rows, Row{Kind: RowSpacer})
		}
		first = false
		qCollapsed := collapsedProjects[q.ID]
		rows = append(rows, Row{Kind: RowLookoutQuest, Label: q.Title, QuestID: q.ID, Collapsed: qCollapsed})
		if qCollapsed {
			continue
		}
		if includeTracks {
			for _, t := range q.Tracks {
				rows = append(rows, Row{Kind: RowTrack, TrackEvent: t.Event, QuestID: q.ID})
			}
		}
		for _, l := range q.Lookouts {
			rows = append(rows, Row{Kind: RowLookout, LookoutURL: l.URL, QuestID: q.ID})
		}
	}
	return rows
}

// VaultOpenKey is the synthetic collapsedProjects key that records whether a
// focused section page's "Vaulted" group is expanded — present = expanded;
// absent = collapsed (the default, so past items stay tucked away).
func VaultOpenKey(section string) string { return "\x00vaultopen:" + section }

// appendVaultedGroups appends a collapsible "Vaulted (N)" sub-header and, when
// expanded, that section's content for parked/archived quests — so a focused
// page can surface past items on demand.
func appendVaultedGroups(s *store.Store, rows []Row, collapsedProjects map[string]bool, section string) []Row {
	n := countVaulted(s, section)
	if n == 0 {
		return rows
	}
	expanded := collapsedProjects[VaultOpenKey(section)]
	rows = append(rows,
		Row{Kind: RowSpacer},
		Row{Kind: RowVaultHeader, Section: section, Label: fmt.Sprintf("Vaulted (%d)", n), Collapsed: !expanded},
	)
	if !expanded {
		return rows
	}
	switch section {
	case "lookouts":
		// Vaulted lookouts only render on the focused page, where tracks show.
		rows = appendAnalyticsGroups(s, rows, collapsedProjects, true, true)
	case "runes":
		rows = appendRuneGroups(s, rows, collapsedProjects, true)
	case "campaigns":
		for i := range s.Projects {
			if p := &s.Projects[i]; p.Archived {
				rows = appendProject(s, rows, *p, collapsedProjects, true, false)
			}
		}
	}
	return rows
}

// countVaulted is how many items a section's Vaulted group would hold.
func countVaulted(s *store.Store, section string) int {
	n := 0
	switch section {
	case "lookouts":
		for i := range s.Quests {
			if q := &s.Quests[i]; questVaulted(s, q) && (len(q.Tracks) > 0 || len(q.Lookouts) > 0) {
				n++
			}
		}
	case "runes":
		for i := range s.Quests {
			if q := &s.Quests[i]; questVaulted(s, q) {
				n += len(q.Runes)
			}
		}
	case "campaigns":
		for i := range s.Projects {
			if s.Projects[i].Archived {
				n++
			}
		}
	}
	return n
}

// CountLookouts is the number of usage dashboards across un-vaulted quests —
// shown in the Tavern's Lookouts section header.
func CountLookouts(s *store.Store) int {
	n := 0
	for i := range s.Quests {
		q := &s.Quests[i]
		if !questVaulted(s, q) {
			n += len(q.Lookouts)
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
		if i > 0 {
			prev := rows[i-1]
			groupStart := r.Kind == RowProject || r.Kind == RowNewProject || r.Kind == RowSection || r.Kind == RowLabel || r.Kind == RowBanner || r.Kind == RowNewBanner
			// A banner's first campaign hugs its header (no separating blank), so
			// the group reads as one block; everything else gets breathing room.
			hugsBanner := (r.Kind == RowProject || r.Kind == RowNewProject) && prev.Kind == RowBanner
			if groupStart && !hugsBanner {
				out = append(out, Row{Kind: RowSpacer})
			}
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
	case "lookouts":
		return "Lookouts", CountLookouts(s)
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
func RenderRow(row Row, s *store.Store, titleView string, isCursor, isNew bool, width int, hint string) (line string, hintX int) {
	cursorMark := "  "
	switch {
	case isCursor:
		cursorMark = StyleCursor.Render(GlyphCursor)
	case isNew:
		// A quest just added via quick-add: a blue dot marks where it landed
		// (until it's selected/opened). Same 2-col slot as the cursor mark.
		cursorMark = StyleNew.Render(GlyphNew)
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
			switch {
			case row.Header:
				// The campaign's own pane title — a tier below the banner header:
				// bold + UPPERCASED in the campaign accent (real-case while editing,
				// which is when titleView is non-empty and skips this branch).
				name = StyleCampaignTitle.Render(strings.ToUpper(p.Name))
			case p.IsCompleted():
				name = StyleDone.Render(p.Name) // done, but stays until you Ctrl+V it
			case row.Dim:
				name = StyleMuted.Render(p.Name) // inactive/later campaign reads dimmer
			default:
				name = StyleName.Render(p.Name)
			}
		}
		done, total := projectProgress(s, p.ID)
		var progress string
		if row.Bare {
			// A drill-in campaign row: a progress ring leads instead of a
			// chevron, and the right shows a plain count. As a pane title the
			// ring picks up the campaign accent so the whole line reads as one.
			ringStyle := StyleMuted
			if row.Header {
				ringStyle = StyleCampaignTitle
			}
			ring := ringStyle.Render(model.ProgressBucket(done, total))
			line = withHint(fmt.Sprintf("%s%s%s %s", cursorMark, nestIndent, ring, name))
			progress = StyleMuted.Render(fmt.Sprintf("%d/%d", done, total))
		} else {
			progress = StyleMuted.Render(fmt.Sprintf("%s %d/%d", model.ProgressBucket(done, total), done, total))
			line = withHint(fmt.Sprintf("%s%s%s %s", cursorMark, nestIndent, caret(row.Collapsed), name))
		}
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
		if row.Dim {
			iconView = StyleMuted.Render(glyph)
		}
		title := titleView
		if title == "" {
			switch {
			case q.Status == model.StatusDone:
				title = StyleDone.Render(q.Title)
			case row.Dim:
				title = StyleMuted.Render(q.Title) // inactive backlog reads dimmer than active work
			default:
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
		// The 2-col slot before the glyph holds the priority arrow (up for
		// medium/high, a muted down-arrow for low), else stays blank — so glyphs
		// stay column-aligned across the list. In the Tavern overview panes
		// (Bare) it's dropped so quests align flush with campaign rows.
		prio := priorityIndicator(q.Priority)
		if row.Bare {
			prio = ""
		}
		return withHint(fmt.Sprintf("%s%s%s%s %s%s%s", cursorMark, nestIndent, prio, iconView, title, tag, progress)), hintX

	case RowSection:
		label, count := sectionInfo(s, row.Section)
		// The Vault reads like an old strongbox: an aged/rusted label with a
		// heavy-door block ornament, rather than a plain live section header.
		if row.Section == "someday" {
			framed := StyleVaultFrame.Render(fmt.Sprintf("▓ %s (%d)", label, count))
			return withHint(fmt.Sprintf("%s%s %s", cursorMark, caret(row.Collapsed), framed)), hintX
		}
		return withHint(StyleSectionHeader.Render(fmt.Sprintf("%s%s %s (%d)", cursorMark, caret(row.Collapsed), label, count))), hintX

	case RowBanner:
		nm := row.Label
		if b := findBanner(s, row.BannerID); b != nil {
			nm = b.Name
		}
		// A Banner reads as an AREA header — bold, accent-colored, UPPERCASED, with
		// a flag emblem — visually a tier above the plain-white campaigns nested
		// under it.
		bannerStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorHeading)
		name := titleView
		if name == "" {
			name = bannerStyle.Render(strings.ToUpper(nm))
		}
		glyph := "\U000f023b" // nf-md-flag (a banner); the banner's own icon overrides
		if b := findBanner(s, row.BannerID); b != nil && b.Icon != "" {
			glyph = b.Icon
		}
		count := StyleMuted.Render(fmt.Sprintf(" (%d)", bannerCampaignCount(s, row.BannerID)))
		if row.Bare {
			// A banner title (not collapsible) — no chevron.
			return withHint(fmt.Sprintf("%s%s %s%s", cursorMark, bannerStyle.Render(glyph), name, count)), hintX
		}
		return withHint(fmt.Sprintf("%s%s %s %s%s", cursorMark, caret(row.Collapsed), bannerStyle.Render(glyph), name, count)), hintX

	case RowNewBanner:
		return cursorMark + StyleMuted.Render("+ New Banner"), -1

	case RowNewProject:
		label := row.Label
		if label == "" {
			label = "+ New Campaign"
		}
		return cursorMark + nestIndent + StyleMuted.Render(label), -1

	case RowNewQuest:
		label := row.Label
		if label == "" {
			label = "+ New Quest"
		}
		return fmt.Sprintf("%s%s%s", cursorMark, nestIndent, StyleMuted.Render(label)), -1

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
		text := StyleMuted.Render(display)
		if titleView != "" {
			text = titleView // live editor when this objective is being renamed
		}
		return withHint(fmt.Sprintf("%s%s%s %s", cursorMark, indent, check, text)), hintX

	case RowRune:
		// titleView is the app-rendered "glyph key  state" content (the live
		// rollout state lives in the app, not here) — indented under its
		// quest header.
		return withHint(fmt.Sprintf("%s  %s", cursorMark, titleView)), hintX

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

	case RowLookout, RowTrack:
		// titleView is the app-rendered "glyph label" content — indented under its
		// quest header.
		return withHint(fmt.Sprintf("%s  %s", cursorMark, titleView)), hintX

	case RowVaultHeader:
		// A muted collapsible "Vaulted (N)" sub-header at the bottom of a focused
		// section page.
		return withHint(cursorMark + StyleMuted.Render(caret(row.Collapsed)+" "+row.Label)), hintX

	case RowLookoutQuest:
		// A collapsible quest header in the Lookouts section, mirroring RowRuneQuest.
		name := row.Label
		if isCursor {
			name = StyleTitle.Render(name)
		} else {
			name = StyleName.Render(name)
		}
		return withHint(fmt.Sprintf("%s%s %s", cursorMark, caret(row.Collapsed), name)), hintX

	case RowDayHeader:
		// Indented past the cursor-mark gutter so a group label ("Later", a
		// Vault day) lines up with the banner title and the items beneath it.
		return "  " + StyleMuted.Render(row.Label), -1

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
