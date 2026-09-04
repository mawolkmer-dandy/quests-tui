package ui

import (
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
)

// The campaigns hall groups campaigns under their Banner, renders a banner
// header + a "+ New Banner" affordance, and lists ungrouped campaigns too.
func TestCampaignRowsGroupByBanner(t *testing.T) {
	s := &store.Store{
		Banners: []model.Banner{{ID: "b1", Name: "Platform"}},
		Projects: []model.Project{
			{ID: "p1", Name: "Migrate", BannerID: "b1"},
			{ID: "p2", Name: "Loose campaign"},
		},
	}
	rows := SectionContent(s, "campaigns", map[string]bool{})

	bannerIdx, p1Idx, p2Idx := -1, -1, -1
	sawNewBanner := false
	for i, r := range rows {
		switch {
		case r.Kind == RowBanner && r.BannerID == "b1":
			bannerIdx = i
		case r.Kind == RowNewBanner:
			sawNewBanner = true
		case r.Kind == RowProject && r.ProjectID == "p1":
			p1Idx = i
		case r.Kind == RowProject && r.ProjectID == "p2":
			p2Idx = i
		}
	}
	if bannerIdx < 0 || p1Idx < 0 || p2Idx < 0 || !sawNewBanner {
		t.Fatalf("missing rows: banner=%d p1=%d p2=%d newBanner=%v", bannerIdx, p1Idx, p2Idx, sawNewBanner)
	}
	if p1Idx < bannerIdx {
		t.Fatalf("the banner's campaign should render below its header (banner=%d, p1=%d)", bannerIdx, p1Idx)
	}
}

// A collapsed banner hides its campaigns (keyed by banner ID in collapsedProjects).
func TestCampaignRowsCollapsedBannerHidesCampaigns(t *testing.T) {
	s := &store.Store{
		Banners:  []model.Banner{{ID: "b1", Name: "Platform"}},
		Projects: []model.Project{{ID: "p1", Name: "Migrate", BannerID: "b1"}},
	}
	rows := SectionContent(s, "campaigns", map[string]bool{"b1": true})
	for _, r := range rows {
		if r.Kind == RowProject && r.ProjectID == "p1" {
			t.Fatal("a collapsed banner must not show its campaigns")
		}
	}
}
