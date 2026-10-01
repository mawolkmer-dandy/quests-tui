package app

import (
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The pinned Errands area is always present and is the FIRST banner in the hall
// — above every real banner, below the rooms (Questboard/Vault). It's the
// deliberate home for campaign-less quests.
func TestErrandsBannerRendersFirst(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store: &store.Store{
			Banners: []model.Banner{{ID: "b1", Name: "Platform"}, {ID: "b2", Name: "Growth"}},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{},
	}

	rows := m.hallRows()
	var bannerOrder []string
	roomsBeforeErrands := 0
	firstBannerSeen := false
	for _, r := range rows {
		if r.Kind == ui.RowSection && !firstBannerSeen {
			roomsBeforeErrands++
		}
		if r.Kind == ui.RowBanner {
			firstBannerSeen = true
			bannerOrder = append(bannerOrder, r.BannerID)
		}
	}

	if len(bannerOrder) == 0 || bannerOrder[0] != errandsBanner {
		t.Fatalf("Errands must be the first banner, got order %v", bannerOrder)
	}
	if roomsBeforeErrands == 0 {
		t.Fatal("the rooms (Questboard/Vault) must still sit above Errands")
	}
	// Real banners follow, in their own order.
	if len(bannerOrder) < 3 || bannerOrder[1] != "b1" || bannerOrder[2] != "b2" {
		t.Fatalf("real banners should follow Errands in order, got %v", bannerOrder)
	}
}

// Errands are ordinary loose quests filed under the pinned area; its pane lists
// them and offers "+ add quest", reusing the real-banner machinery.
func TestErrandsPaneListsLooseQuests(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store: &store.Store{
			Quests: []model.Quest{
				{ID: "e1", Title: "Sign compliance form", BannerID: errandsBanner, Status: model.StatusOpen},
			},
		},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}

	rows := m.bannerPaneRows(errandsBanner)
	header, quest, addQuest, addCampaign := false, false, false, false
	for _, r := range rows {
		switch {
		case r.Kind == ui.RowBanner && r.BannerID == errandsBanner && r.Label == errandsLabel:
			header = true
		case r.Kind == ui.RowQuest && r.QuestID == "e1":
			quest = true
		case r.Kind == ui.RowNewQuest:
			addQuest = true
		case r.Kind == ui.RowNewProject:
			addCampaign = true
		}
	}
	if !header {
		t.Fatal("the Errands pane header should carry the synthetic 'Errands' label")
	}
	if !quest {
		t.Fatal("an errand should render in the Errands pane")
	}
	if addCampaign {
		t.Fatal("Errands must not offer '+ add campaign' — quests only")
	}
	_ = addQuest // "+ add quest" only shows when empty; here there's already an errand
}

// The Errands area, holding only loose quests, adds a quest (not a campaign) on
// Enter; its badge counts errands, not campaigns.
func TestErrandsAddsQuestsNotCampaigns(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store:             &store.Store{},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}
	projectsBefore := len(m.store.Projects)
	m.hallCursor = cursorTarget{kind: ui.RowBanner, bannerID: errandsBanner}
	m.createCampaignFromHall(errandsBanner) // Enter on the Errands hall banner

	if len(m.store.Projects) != projectsBefore {
		t.Fatal("adding under Errands must not create a campaign")
	}
	loose := 0
	for _, q := range m.store.Quests {
		if q.ProjectID == "" && q.BannerID == errandsBanner {
			loose++
		}
	}
	if loose != 1 {
		t.Fatalf("Enter on Errands should add one loose quest, got %d", loose)
	}
	if got := ui.BannerItemCount(m.store, errandsBanner); got != 1 {
		t.Fatalf("Errands badge should count its 1 errand, got %d", got)
	}
}

// A normal area's badge counts campaigns AND its loose quests.
func TestBannerItemCountIncludesLooseQuests(t *testing.T) {
	s := &store.Store{
		Projects: []model.Project{
			{ID: "p1", Name: "A", BannerID: "b1"},
			{ID: "p2", Name: "B", BannerID: "b1"},
			{ID: "arch", Name: "Old", BannerID: "b1", Archived: true}, // excluded
		},
		Quests: []model.Quest{
			{ID: "lq", Title: "loose", BannerID: "b1"},                          // counted
			{ID: "vq", Title: "vaulted", BannerID: "b1", Vaulted: true},         // excluded
			{ID: "dq", Title: "done", BannerID: "b1", Status: model.StatusDone}, // excluded (finished)
			{ID: "cq", Title: "in campaign", ProjectID: "p1", BannerID: "b1"},   // excluded (has campaign)
		},
	}
	if got := ui.BannerItemCount(s, "b1"); got != 3 { // 2 campaigns + 1 open loose quest
		t.Fatalf("BannerItemCount = %d, want 3 (2 campaigns + 1 open loose quest)", got)
	}
}

// The footer surfaces Ctrl+E (schedule) whenever a quest is selected, and the
// Errands area's add-verb is "quest" (not "campaign").
func TestFooterHints(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store:             &store.Store{},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}
	has := func(parts []hintPart, key string) bool {
		for _, p := range parts {
			if p.key == key {
				return true
			}
		}
		return false
	}

	// A selected quest in the pane exposes the schedule shortcut.
	m.hallFocus = false
	m.cursor = cursorTarget{kind: ui.RowQuest, questID: "q"}
	if !has(m.tavernHint(), "ctrl+e") {
		t.Error("a selected quest's footer should offer ctrl+e schedule")
	}

	// The Errands hall banner adds a quest, not a campaign.
	m.hallFocus = true
	m.hallCursor = cursorTarget{kind: ui.RowBanner, bannerID: errandsBanner}
	verbHasQuest := false
	for _, p := range m.tavernHint() {
		if p.key == "enter" && p.verb == "new quest" {
			verbHasQuest = true
		}
	}
	if !verbHasQuest {
		t.Errorf("Errands hall banner should add a quest: %+v", m.tavernHint())
	}
}

// The pinned Errands area is synthetic (not in Store.Banners), so it resists
// deletion — opening the delete dialog on it is a no-op, exactly like Unassigned.
func TestErrandsBannerCannotBeDeleted(t *testing.T) {
	ui.Init(true)
	m := &Model{
		store:             &store.Store{},
		path:              filepath.Join(t.TempDir(), "data.json"),
		collapsedProjects: map[string]bool{},
	}
	if m.openDeleteModal(cursorTarget{kind: ui.RowBanner, bannerID: errandsBanner}) {
		t.Fatal("the Errands area must not be deletable")
	}
	if m.modal != nil {
		t.Fatal("no delete dialog should open for the Errands area")
	}
}
