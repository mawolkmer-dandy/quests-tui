package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func copyModel() *Model {
	return &Model{
		jiraBaseURL: "https://meetdandy.atlassian.net",
		ldProject:   "default",
		ldEnv:       "production",
		prStatus: map[string]PRStatus{
			"#48709": {Code: "#48709", Status: "success", Title: "painted-door step", Additions: 1175, Deletions: 39},
			"#48744": {Code: "#48744", Status: "merged", Title: "reset once-per-doctor history", Additions: 27, Deletions: 1},
			"#70":    {Code: "#70", Status: "success", Title: "poller", Additions: 40, Deletions: 2},
		},
	}
}

func TestPRShareLineFormat(t *testing.T) {
	m := copyModel()
	got := m.prShareLine(model.PRLink{Code: "#48709", Repo: "orthly/orthlyweb"}, "orthly/orthlyweb")
	want := "[#48709 painted-door step](https://github.com/orthly/orthlyweb/pull/48709) `+1175/-39`"
	if got != want {
		t.Fatalf("open PR line = %q, want %q", got, want)
	}
	got = m.prShareLine(model.PRLink{Code: "#48744", Repo: "orthly/orthlyweb"}, "orthly/orthlyweb")
	if got != "[#48744 reset once-per-doctor history](https://github.com/orthly/orthlyweb/pull/48744) `+27/-1`" {
		t.Fatalf("merged PR line = %q", got)
	}
	// Cross-repo → owner/repo#code in the link text.
	got = m.prShareLine(model.PRLink{Code: "#70", Repo: "orthly/unified-practice-uploader"}, "orthly/orthlyweb")
	if got != "[orthly/unified-practice-uploader#70 poller](https://github.com/orthly/unified-practice-uploader/pull/70) `+40/-2`" {
		t.Fatalf("cross-repo PR line = %q", got)
	}
}

func TestCopySectionTrails(t *testing.T) {
	m := copyModel()
	q := &model.Quest{PRs: []model.PRLink{
		{Code: "#48709", Repo: "orthly/orthlyweb"},
		{Code: "#48744", Repo: "orthly/orthlyweb"},
	}}
	got := m.copySection(q, secTrails)
	want := "[#48709 painted-door step](https://github.com/orthly/orthlyweb/pull/48709) `+1175/-39`\n" +
		"[#48744 reset once-per-doctor history](https://github.com/orthly/orthlyweb/pull/48744) `+27/-1`"
	if got != want {
		t.Fatalf("trails section:\n%q\nwant:\n%q", got, want)
	}
}

func TestCopySectionScrollsMarkdownLink(t *testing.T) {
	m := copyModel()
	m.jiraStatus = map[string]JiraStatus{
		"EPDCHAIR-5711": {Code: "EPDCHAIR-5711", Title: "Introduce a scanner overlay window"},
	}
	q := &model.Quest{JiraCodes: []string{"EPDCHAIR-5711", "EPDCHAIR-5734"}}
	got := m.copySection(q, secScrolls)
	want := "[EPDCHAIR-5711 Introduce a scanner overlay window](https://meetdandy.atlassian.net/browse/EPDCHAIR-5711)\n" +
		"[EPDCHAIR-5734](https://meetdandy.atlassian.net/browse/EPDCHAIR-5734)" // no summary cached → key only in the link text
	if got != want {
		t.Fatalf("scrolls section:\n%q", got)
	}
}

func TestCopySectionRunesMarkdownLink(t *testing.T) {
	m := copyModel()
	q := &model.Quest{Runes: []string{"scanneros_disable_third_window_overlay"}}
	got := m.copySection(q, secRunes)
	want := "[scanneros_disable_third_window_overlay](https://app.launchdarkly.com/projects/default/flags/scanneros_disable_third_window_overlay/targeting?env=production&selected-env=production)"
	if got != want {
		t.Fatalf("runes section = %q", got)
	}
}

func TestCopySectionTracksWithMarks(t *testing.T) {
	m := copyModel()
	q := &model.Quest{Tracks: []model.Track{
		{Event: "Practice - Chairside - Scanner Overlay Shown", Marks: []string{"scannerType"}},
		{Event: "No Marks Event"},
	}}
	got := m.copySection(q, secTracks)
	want := "Practice - Chairside - Scanner Overlay Shown\n  `{ scannerType }`\nNo Marks Event"
	if got != want {
		t.Fatalf("tracks section:\n%q", got)
	}
}

func TestCopyItemSingleLink(t *testing.T) {
	m := copyModel()
	q := &model.Quest{}
	text, _ := m.copyItem(q, focusLink{kind: linkPR, code: "#48709", url: "https://github.com/orthly/orthlyweb/pull/48709"})
	if text != "https://github.com/orthly/orthlyweb/pull/48709" {
		t.Fatalf("single PR copy = %q", text)
	}
}

func TestClickLinkCopyThenOpen(t *testing.T) {
	m := &Model{}
	url := "https://example.com/x"
	if cmd := m.clickLink(url); cmd == nil {
		t.Fatal("first click should copy (non-nil toast cmd)")
	}
	if m.lastLinkClickURL != url {
		t.Fatal("first click should arm the double-click")
	}
	// A second click on the SAME url within the window opens (disarms).
	m.clickLink(url)
	if m.lastLinkClickURL != "" {
		t.Fatal("second click should disarm after opening")
	}
	// A stale second click (outside the window) copies again, not opens.
	m.clickLink(url)
	m.lastLinkClickAt = m.lastLinkClickAt.Add(-time.Second)
	m.clickLink(url)
	if m.lastLinkClickURL != url {
		t.Fatal("a click outside the window should re-arm (copy), not open")
	}
}

func TestShortenURL(t *testing.T) {
	got := shortenURL("https://docs.google.com/document/d/1FmbgLVtTtBy1-Rn1S9GQyNTRg6pE7aYVmwa6n36ravM/edit?tab=t.1ije8he7oboj#heading=h.lm245gvhl65p")
	want := "docs.google.com/document/d/1FmbgLVtTtBy…/edit?tab=…#heading=…"
	if got != want {
		t.Fatalf("shortenURL:\n got  %q\n want %q", got, want)
	}
	// A short URL is left essentially intact (scheme/www dropped only).
	if got := shortenURL("https://www.example.com/hi"); got != "example.com/hi" {
		t.Fatalf("short url = %q", got)
	}
}

func TestShortenBodyLinksCapture(t *testing.T) {
	m := &Model{}
	q := &model.Quest{}
	url := "https://docs.google.com/document/d/1FmbgLVtTtBy1-Rn1S9GQyNTRg6pE7aYVmwa6n36ravM/edit"
	out, changed := m.shortenBodyLinks(q, "see "+url+" thanks")
	short := "docs.google.com/document/d/1FmbgLVtTtBy…/edit"
	if !changed {
		t.Fatal("shortenBodyLinks should report a change when a URL is shortened")
	}
	if out != "see "+short+" thanks" {
		t.Fatalf("shortened body = %q", out)
	}
	if q.BodyLinks[short] != url {
		t.Fatalf("BodyLinks not recorded: %+v", q.BodyLinks)
	}
	// Trailing sentence punctuation stays outside the link.
	q2 := &model.Quest{}
	out, _ = m.shortenBodyLinks(q2, "(https://www.example.com/a/b).")
	if out != "(example.com/a/b)." {
		t.Fatalf("punctuation handling = %q", out)
	}
	// No URL → no change (so a typed trailing space is never eaten).
	if out, changed := m.shortenBodyLinks(&model.Quest{}, "hello world "); changed || out != "hello world " {
		t.Fatalf("plain text should be untouched: %q changed=%v", out, changed)
	}
}

func TestCaptureAndStripPreservesTrailingSpace(t *testing.T) {
	m := &Model{}
	q := &model.Quest{}
	// Plain text with a trailing space and a double space: no link → no change,
	// so the caller won't reseed and eat the just-typed space.
	if _, _, _, _, _, changed := m.captureAndStrip(q, "hello world "); changed {
		t.Fatal("plain trailing-space line must report changed=false")
	}
	// A real URL still reports changed (so it shortens/captures).
	if _, _, _, _, _, changed := m.captureAndStrip(q, "see https://docs.google.com/x/y here"); !changed {
		t.Fatal("a line with a URL must report changed=true")
	}
}

func TestLookoutRename(t *testing.T) {
	url := "https://app.amplitude.com/analytics/orthly/dashboard/kl3swjdw"
	q := &model.Quest{ID: "q1", Lookouts: []model.Lookout{{URL: url, Tool: "amplitude"}}}
	m := &Model{store: &store.Store{Quests: []model.Quest{*q}}}
	q = &m.store.Quests[0]

	// Derived label until named.
	if got := lookoutLabel(q.Lookouts[0]); got != "Amplitude · kl3swjdw" {
		t.Fatalf("derived label = %q", got)
	}
	m.beginLookoutRename(q.ID, url)
	if m.lookoutEditor == nil {
		t.Fatal("rename did not open an editor")
	}
	m.lookoutEditor.SetValue("Smokescreen adoption")
	m.commitLookoutRename()
	if m.lookoutEditor != nil {
		t.Fatal("commit did not close the editor")
	}
	if q.Lookouts[0].Label != "Smokescreen adoption" {
		t.Fatalf("label = %q", q.Lookouts[0].Label)
	}
	if got := lookoutLabel(q.Lookouts[0]); got != "Smokescreen adoption" {
		t.Fatalf("named label = %q", got)
	}
	// Committing empty clears it back to the derived name.
	m.beginLookoutRename(q.ID, url)
	m.lookoutEditor.SetValue("   ")
	m.commitLookoutRename()
	if q.Lookouts[0].Label != "" {
		t.Fatalf("blank rename should clear the label, got %q", q.Lookouts[0].Label)
	}
}

func TestLookoutRenameFromTavernKey(t *testing.T) {
	url := "https://app.amplitude.com/x/y"
	m := &Model{
		store:             &store.Store{Quests: []model.Quest{{ID: "q1", Lookouts: []model.Lookout{{URL: url}}}}},
		collapsedProjects: map[string]bool{},
		collapsedSections: map[string]bool{},
	}
	m.cursor = cursorTarget{kind: ui.RowLookout, questID: "q1", lookoutURL: url}

	// "r" on a focused Lookout row opens the rename editor (Tavern path).
	m.handleRowKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.lookoutEditor == nil {
		t.Fatal("'r' in the Tavern did not open the rename editor")
	}
	// Shared key handler commits on Enter.
	m.lookoutEditor.SetValue("Funnel")
	if _, handled := m.handleLookoutRenameKey(tea.KeyPressMsg{Code: tea.KeyEnter}); !handled {
		t.Fatal("Enter not handled by the shared rename key path")
	}
	if got := m.store.Quests[0].Lookouts[0].Label; got != "Funnel" {
		t.Fatalf("label = %q", got)
	}
}

func TestCopyTrackIndividually(t *testing.T) {
	ev := "Practice - Chairside - Scanner Overlay Shown"
	st := &store.Store{Quests: []model.Quest{{ID: "q1", Tracks: []model.Track{{Event: ev, Marks: []string{"scannerType"}}}}}}
	m := &Model{store: st}
	// copyTrack builds the event + marks snippet (the "individual" copy).
	if cmd := m.copyTrack("q1", ev); cmd == nil {
		t.Fatal("copyTrack returned nil for a real track")
	}
	// The Tavern click path copies too (RowTrack is no longer a dead info row).
	cmd, ok := m.commonRowClick(ui.Row{Kind: ui.RowTrack, QuestID: "q1", TrackEvent: ev})
	if !ok || cmd == nil {
		t.Fatalf("RowTrack click should copy: ok=%v cmd=%v", ok, cmd)
	}
	// The "c" key on a selected track copies (Tavern/section path).
	m.cursor = cursorTarget{kind: ui.RowTrack, questID: "q1", trackEvent: ev}
	if cmd := m.handleRowKey(tea.KeyPressMsg{Code: 'c', Text: "c"}); cmd == nil {
		t.Fatal("'c' on a selected track should copy")
	}
}

func TestFocusDownReachesLastStop(t *testing.T) {
	m := &Model{modal: &Modal{Kind: ModalQuestDetail, QuestID: "q1"}}
	q := &model.Quest{ID: "q1"}
	// Simulate a render's focus stops: section headers interleaved with items —
	// exactly the case focusLinkCount undercounted.
	m.focusLinks = []focusLink{
		{line: 0, kind: linkCopySection, code: secTrails},
		{line: 1, kind: linkPR, code: "#1"},
		{line: 2, kind: linkCopySection, code: secLookouts},
		{line: 3, kind: linkLookout, code: "u"},
	}
	m.focusLinkIdx = 0
	for i := 0; i < 8; i++ { // more presses than stops — must not overshoot
		m.handleFocusLinkKey(tea.KeyPressMsg{Code: tea.KeyDown}, q)
	}
	if m.focusLinkIdx != len(m.focusLinks)-1 {
		t.Fatalf("Down settled at %d, want last stop %d", m.focusLinkIdx, len(m.focusLinks)-1)
	}
	// And Up walks back to the top.
	for i := 0; i < 8; i++ {
		if m.focusLinkIdx == 0 {
			break
		}
		m.handleFocusLinkKey(tea.KeyPressMsg{Code: tea.KeyUp}, q)
	}
	if m.focusLinkIdx != 0 {
		t.Fatalf("Up settled at %d, want 0", m.focusLinkIdx)
	}
}

func TestBodyLinksRoundTrip(t *testing.T) {
	q := model.Quest{ID: "q1", BodyLinks: map[string]string{"docs.google.com/d/x…": "https://docs.google.com/d/xyz"}}
	st := &store.Store{Quests: []model.Quest{q}}
	dir := t.TempDir()
	path := dir + "/data.json"
	if err := store.Save(path, st); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Quests[0].BodyLinks["docs.google.com/d/x…"] != "https://docs.google.com/d/xyz" {
		t.Fatalf("BodyLinks lost on round-trip: %+v", got.Quests[0].BodyLinks)
	}
}
