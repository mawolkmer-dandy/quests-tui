package app

import (
	"context"
	"fmt"
	"image/color"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/atotto/clipboard"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// Tracks are harvested tracking events (event name + property "marks"); Lookouts
// are the per-quest usage dashboards you monitor them in. Tracks come only from
// harvesting a quest's linked PRs (see harvest.go) — never typed by hand.
// Lookouts are captured by pasting a dashboard URL. A quest's incantation — an
// on-demand, cheap-Claude prompt for building a dashboard — is generated from
// ALL of its Tracks at once.

// --- lookups / mutation -------------------------------------------------

func findTrack(q *model.Quest, event string) (*model.Track, int) {
	for i := range q.Tracks {
		if q.Tracks[i].Event == event {
			return &q.Tracks[i], i
		}
	}
	return nil, -1
}

func lookoutIndexOf(ls []model.Lookout, url string) int {
	for i := range ls {
		if ls[i].URL == url {
			return i
		}
	}
	return -1
}

func findLookout(q *model.Quest, url string) (*model.Lookout, int) {
	if i := lookoutIndexOf(q.Lookouts, url); i >= 0 {
		return &q.Lookouts[i], i
	}
	return nil, -1
}

// dismissTrack removes a track and remembers its event so auto-harvest won't
// bring it back.
func (m *Model) dismissTrack(questID, event string) {
	q := m.findQuest(questID)
	if q == nil {
		return
	}
	out := q.Tracks[:0]
	for _, t := range q.Tracks {
		if t.Event != event {
			out = append(out, t)
		}
	}
	q.Tracks = out
	if indexOfStr(q.DismissedTracks, event) < 0 {
		q.DismissedTracks = append(q.DismissedTracks, event)
	}
	q.UpdatedAt = time.Now()
	m.save()
}

// removeLookout drops a dashboard from a quest.
func (m *Model) removeLookout(questID, url string) {
	q := m.findQuest(questID)
	if q == nil {
		return
	}
	out := q.Lookouts[:0]
	for _, l := range q.Lookouts {
		if l.URL != url {
			out = append(out, l)
		}
	}
	q.Lookouts = out
	q.UpdatedAt = time.Now()
	m.save()
}

// openLookoutScry opens a lookout's dashboard URL.
func (m *Model) openLookoutScry(url string) tea.Cmd {
	if url == "" {
		return nil
	}
	return openURL(url)
}

// --- tool / labels ------------------------------------------------------

// inferTool guesses which dashboard tool a URL belongs to from its host.
func inferTool(url string) string {
	u := strings.ToLower(url)
	switch {
	case strings.Contains(u, "amplitude"):
		return "amplitude"
	case strings.Contains(u, "fullstory"):
		return "fullstory"
	case strings.Contains(u, "hex.tech"), strings.Contains(u, "app.hex"):
		return "hex"
	}
	return ""
}

func toolLabel(tool string) string {
	switch tool {
	case "amplitude":
		return "Amplitude"
	case "fullstory":
		return "Fullstory"
	case "hex":
		return "Hex"
	}
	return "your analytics tool"
}

// lookoutLabel is a lookout's display name: its custom label, else the tool
// plus a short tail of the URL to disambiguate multiple dashboards.
func lookoutLabel(l model.Lookout) string {
	if l.Label != "" {
		return l.Label
	}
	name := toolLabel(l.Tool)
	if tail := urlTail(l.URL); tail != "" {
		name += " · " + tail
	}
	return name
}

// beginLookoutRename opens an inline editor over a Lookout's label, seeded with
// its current custom label (blank if it only has the derived one). Keyed on the
// quest id + URL so it works from any view (Tavern rail, section page, detail).
func (m *Model) beginLookoutRename(questID, url string) {
	q := m.findQuest(questID)
	if q == nil {
		return
	}
	cur := ""
	for _, l := range q.Lookouts {
		if l.URL == url {
			cur = l.Label
			break
		}
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(cur)
	ti.CursorEnd()
	_ = ti.Focus()
	m.lookoutEditor = &ti
	m.lookoutEditURL = url
	m.lookoutEditQuestID = questID
	m.clearSelection()
	m.invalidateRender()
}

// commitLookoutRename writes the editor value to the Lookout's Label (an empty
// value clears it, falling back to the derived "Tool · id" name) and closes.
func (m *Model) commitLookoutRename() {
	if m.lookoutEditor == nil {
		return
	}
	label := strings.TrimSpace(m.lookoutEditor.Value())
	if q := m.findQuest(m.lookoutEditQuestID); q != nil {
		for i := range q.Lookouts {
			if q.Lookouts[i].URL == m.lookoutEditURL {
				q.Lookouts[i].Label = label
				break
			}
		}
		m.save()
	}
	m.cancelLookoutRename()
}

// cancelLookoutRename closes the inline Lookout editor without saving.
func (m *Model) cancelLookoutRename() {
	m.lookoutEditor = nil
	m.lookoutEditURL = ""
	m.lookoutEditQuestID = ""
	m.clearSelection()
	m.invalidateRender()
}

// handleLookoutRenameKey routes a key to the inline Lookout editor while it's
// open — shared by every view (the Tavern's handleKey and the detail modal's
// updateModal) so renaming behaves identically everywhere.
func (m *Model) handleLookoutRenameKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.lookoutEditor == nil {
		return nil, false
	}
	switch msg.Code {
	case tea.KeyEsc:
		m.cancelLookoutRename()
		return nil, true
	case tea.KeyEnter:
		m.commitLookoutRename()
		return nil, true
	}
	if handled, cmd := m.applySelectionKey(m.lookoutEditor, msg); handled {
		return cmd, true
	}
	var cmd tea.Cmd
	*m.lookoutEditor, cmd = m.lookoutEditor.Update(msg)
	return cmd, true
}

// lookoutRowTitle is a Lookout row's title text — the inline editor while it's
// being renamed, else its normal label. One definition, used by every view.
func (m *Model) lookoutRowTitle(questID, url string) string {
	if m.lookoutEditor != nil && m.lookoutEditURL == url {
		return m.renderEditableStyled(m.lookoutEditor, ui.StyleName)
	}
	return m.lookoutRowContent(questID, url)
}

// urlTail is a short identifier from a URL — its last path segment (query
// dropped, trimmed) — for telling two same-tool dashboards apart.
func urlTail(url string) string {
	u := url
	if i := strings.Index(u, "?"); i >= 0 {
		u = u[:i]
	}
	u = strings.TrimRight(u, "/")
	if i := strings.LastIndex(u, "/"); i >= 0 {
		u = u[i+1:]
	}
	if len(u) > 16 {
		u = u[:16]
	}
	return u
}

// --- glyphs / row content ----------------------------------------------

// trackProdColor colors a track by whether the PR that introduced it is merged
// (live in production → green), still open (pending → amber), or unknown (tan).
func (m *Model) trackProdColor(sourcePR string) color.Color {
	st, ok := m.prStatus[sourcePR]
	switch {
	case ok && st.Status == "merged":
		return ui.ColorHeading
	case ok:
		return ui.ColorPriorityMedium
	default:
		return ui.ColorTrack
	}
}

// trackGlyph is a track's footprint, colored by its production state.
func (m *Model) trackGlyph(t model.Track) string {
	return lipgloss.NewStyle().Foreground(m.trackProdColor(t.SourcePR)).Render(ui.GlyphConnTrack)
}

func lookoutGlyph() string {
	return lipgloss.NewStyle().Foreground(ui.ColorLookout).Render(ui.GlyphConnLookout)
}

// staleDays / veryStaleDays are the ages (in days live in production) past
// which a flag's day count is tinted muted-yellow, then muted-red — a nudge
// that a feature flag has been live long enough to clean up.
const (
	staleDays     = 30
	veryStaleDays = 90
)

// ageDaysLabel is the trailing "  Nd" age suffix (or "" when the age is
// unknown). For flags (colored) the count is muted-yellow past staleDays and
// muted-red past veryStaleDays; every other item renders it plain muted. ONE
// definition so every surface ages items identically (see docs/ui-consistency.md).
func ageDaysLabel(days int, colored bool) string {
	if days < 0 {
		return ""
	}
	style := ui.StyleMuted
	if colored {
		switch {
		case days > veryStaleDays:
			style = lipgloss.NewStyle().Faint(true).Foreground(ui.ColorImportant)
		case days > staleDays:
			style = lipgloss.NewStyle().Faint(true).Foreground(ui.ColorPriorityMedium)
		}
	}
	return "  " + style.Render(fmt.Sprintf("%dd", days))
}

// agoStr is a compact relative age ("just now" / "5m ago" / "3h ago" /
// "4d ago"), falling back to an absolute short date past a week. Empty for a
// zero time.
func agoStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// daysSince returns whole days since t, or -1 when t is zero (unknown).
func daysSince(t time.Time) int {
	if t.IsZero() {
		return -1
	}
	d := int(time.Since(t).Hours() / 24)
	if d < 0 {
		d = 0
	}
	return d
}

// mergeAgeDays is how many days a track's source PR has been merged (in
// production), or -1 if that PR isn't merged / unknown.
func (m *Model) mergeAgeDays(sourcePR string) int {
	st, ok := m.prStatus[sourcePR]
	if !ok || st.Status != "merged" {
		return -1
	}
	return daysSince(st.MergedAt)
}

// trackWord is the muted status beside a track: its mark count and, once its
// source PR is merged, its age in days ("Nd" — no coloring; tracks are
// informational, only flags get the stale tint).
func (m *Model) trackWord(t model.Track) string {
	status := "pending"
	if d := m.mergeAgeDays(t.SourcePR); d >= 0 {
		status = fmt.Sprintf("%dd", d)
	}
	if n := len(t.Marks); n > 0 {
		return fmt.Sprintf("%d marks · %s", n, status)
	}
	return status
}

// trackRowContent is the "glyph event  status" text for one track row in the
// Lookouts section / focused page.
func (m *Model) trackRowContent(questID, event string) string {
	q := m.findQuest(questID)
	if q == nil {
		return event
	}
	t, idx := findTrack(q, event)
	if idx < 0 {
		return event
	}
	return m.trackGlyph(*t) + " " + event + "  " + ui.StyleMuted.Render(m.trackWord(*t))
}

// lookoutRowContent is the "glyph label" text for one lookout in the Tavern's
// Lookouts section.
func (m *Model) lookoutRowContent(questID, url string) string {
	q := m.findQuest(questID)
	if q == nil {
		return url
	}
	l, idx := findLookout(q, url)
	if idx < 0 {
		return url
	}
	return lookoutGlyph() + " " + lookoutLabel(*l) + ageDaysLabel(daysSince(l.AddedAt), false)
}

// --- Lookout plans (per-quest dashboard build-plan) --------------------

// plansMsg carries the written Lookout plans back into Update to be copied.
type plansMsg struct {
	questID string
	text    string
}

const plansTimeout = 30 * time.Second

// forgePlans writes the Lookout's plans — a dashboard-building prompt covering
// ALL of a quest's Tracks — on demand, via a cheap Claude model, with a
// static-template fallback if the CLI is missing or errors. Never runs on its
// own.
func (m *Model) forgePlans(q *model.Quest) tea.Cmd {
	if len(q.Tracks) == 0 {
		return m.showClipboardToastText("no tracks yet — harvest a trail first")
	}
	m.plansBusyQuest = q.ID
	m.invalidateRender()
	prompt := plansMetaPrompt(q)
	fallback := staticPlans(q)
	id := q.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), plansTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, "claude", "-p", prompt, "--model", "haiku").Output()
		text := strings.TrimSpace(string(out))
		if err != nil || text == "" {
			text = fallback
		}
		return plansMsg{questID: id, text: text}
	}
}

// applyPlans copies the written Lookout plans to the clipboard with a toast.
func (m *Model) applyPlans(msg plansMsg) tea.Cmd {
	if m.plansBusyQuest == msg.questID {
		m.plansBusyQuest = ""
	}
	_ = clipboard.WriteAll(msg.text)
	return m.showClipboardToastText("Lookout plans copied → paste into your dashboard AI")
}

// plansTool is the tool the quest's first lookout uses (so the prompt is
// tailored), or "" when it has no lookout yet.
func plansTool(q *model.Quest) string {
	for _, l := range q.Lookouts {
		if l.Tool != "" {
			return l.Tool
		}
	}
	return ""
}

func plansMetaPrompt(q *model.Quest) string {
	tool := toolLabel(plansTool(q))
	var b strings.Builder
	b.WriteString("You are helping a product engineer build one analytics usage-monitoring dashboard for a feature. ")
	fmt.Fprintf(&b, "Write a single, concise, copy-paste-ready prompt they can paste into %s's AI assistant to build it. ", tool)
	b.WriteString("The dashboard should monitor these tracking events (with the properties worth breaking each down by):\n")
	for _, t := range q.Tracks {
		marks := "none"
		if len(t.Marks) > 0 {
			marks = strings.Join(t.Marks, ", ")
		}
		fmt.Fprintf(&b, "- %q (properties: %s)\n", t.Event, marks)
	}
	b.WriteString("Cover: total and unique volume per event over the last 30 and 90 days (daily + weekly); segmentation by the listed properties; a funnel or retention view across the events where it makes sense; and a flag for any event whose volume drops to zero recently (a deprecation signal). ")
	if plansTool(q) == "hex" {
		b.WriteString("Since this targets Hex, also include starter BigQuery SQL selecting these events from the events table. ")
	}
	b.WriteString("Output ONLY the prompt text to paste — no preamble, no markdown fences.")
	return b.String()
}

func staticPlans(q *model.Quest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Build a usage-monitoring dashboard in %s covering these tracking events:\n\n", toolLabel(plansTool(q)))
	for _, t := range q.Tracks {
		marks := "(no specific properties)"
		if len(t.Marks) > 0 {
			marks = strings.Join(t.Marks, ", ")
		}
		fmt.Fprintf(&b, "- %s — break down by: %s\n", t.Event, marks)
	}
	b.WriteString("\nInclude, per event: total and unique volume over the last 30 and 90 days (daily + weekly); segmentation by the properties above; a funnel or retention view where relevant; and a flag for any event whose recent volume dropped to zero (deprecation signal).")
	if plansTool(q) == "hex" {
		b.WriteString("\nAlso include starter BigQuery SQL selecting these events from the events table.")
	}
	return b.String()
}
