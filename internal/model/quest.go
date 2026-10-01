package model

import (
	"strings"
	"time"
)

type QuestType string

const (
	QuestTypeMain QuestType = "main"
	QuestTypeSide QuestType = "side"
)

// QuestStatus is a single mutually-exclusive state. Note this "Active" is
// distinct from the app-wide sense of "active" used elsewhere (any quest
// under a non-archived campaign, regardless of status) — this one is an
// explicit "I'm currently working on this" marker the user sets themselves.
// Being vaulted (see Quest.Vaulted) is a separate, orthogonal axis — a
// vaulted quest keeps whatever status it had before it was parked.
type QuestStatus string

const (
	StatusOpen   QuestStatus = ""
	StatusActive QuestStatus = "active"
	StatusDone   QuestStatus = "done"
)

// Priority is an optional emphasis on a quest, orthogonal to type/status.
// Medium and High float to the top (when priority_to_top is on); Low is a
// deprioritization marker (a muted down-arrow) and doesn't float. Cycled with
// the priority key: none → medium → high → low → none.
type Priority string

const (
	PriorityNone   Priority = ""
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityLow    Priority = "low"
)

type BodyLineKind string

const (
	BodyText      BodyLineKind = "text"
	BodyHeading   BodyLineKind = "heading"
	BodyObjective BodyLineKind = "objective"
)

// BodyLine is one line of a quest's outline. Text is the raw text as typed,
// including any "# "/"- " prefix — Kind is always derived from it live via
// ClassifyBodyLine rather than stored, so it can never drift out of sync
// with what's actually on the line.
type BodyLine struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Done   bool   `json:"done"`             // only meaningful when the line classifies as BodyObjective
	Indent int    `json:"indent,omitempty"` // nesting depth (0 = top level); Tab/Shift+Tab adjust it
}

// ClassifyBodyLine derives a line's kind and display text (prefix stripped)
// from its raw typed text: "# " starts a heading, "- " starts an objective,
// anything else is plain text.
func ClassifyBodyLine(text string) (kind BodyLineKind, display string) {
	switch {
	case strings.HasPrefix(text, "# "):
		return BodyHeading, strings.TrimPrefix(text, "# ")
	case strings.HasPrefix(text, "- "):
		return BodyObjective, strings.TrimPrefix(text, "- ")
	default:
		return BodyText, text
	}
}

type Quest struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	Type      QuestType   `json:"type"`
	Status    QuestStatus `json:"status"`
	Vaulted   bool        `json:"vaulted"`
	Priority  Priority    `json:"priority,omitempty"`  // optional emphasis, shown with a left arrow; orthogonal to type/status
	Important bool        `json:"important,omitempty"` // deprecated: migrated to Priority=High on load
	ProjectID string      `json:"projectId"`
	// BannerID, set together with an empty ProjectID, marks a quest that lives
	// loose directly under a Banner (ongoing area work, not part of any campaign).
	BannerID    string     `json:"bannerId,omitempty"`
	Body        []BodyLine `json:"body"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	VaultedAt   *time.Time `json:"vaultedAt,omitempty"` // when the quest was moved to the Vault; drives the Vault's day timeline

	// Muster is the quest's scheduled "when" date — the day it reports to Camp. A
	// quest whose Muster is today or earlier surfaces in Camp as a "called" quest
	// (still inactive until taken up); nil = unscheduled. It is NOT a deadline;
	// stored date-only. For a rite (see Recurrence), Muster is the next occurrence
	// and advances each cycle.
	Muster *time.Time `json:"muster,omitempty"`

	// Recurrence, when set, makes this a "rite" — a recurring quest that recycles
	// its Muster to the next occurrence on completion instead of finishing once.
	Recurrence *Recurrence `json:"recurrence,omitempty"`

	// Integration links, captured from URLs pasted into the body (see
	// internal/model/links.go and internal/app/links.go). JiraCodes holds every
	// linked Jira issue and PRs every linked GitHub PR, each in the order it was
	// captured. All omitempty.
	JiraCodes []string `json:"jiraCodes,omitempty"` // every linked Jira issue, e.g. "EPDCHAIR-5713"
	PRs       []PRLink `json:"prs,omitempty"`       // every linked GitHub PR

	// AgentWorkspaces pins herdr agents (by terminal id, e.g. "term_6579…") to
	// this quest, so each pinned agent's live herdr state shows on the quest.
	// Added from the "+ add Claude agent" affordance (see internal/app/agents.go).
	// The JSON key stays "agentWorkspaces" for back-compat with saved data.
	AgentWorkspaces []string `json:"agentWorkspaces,omitempty"`

	// Runes are LaunchDarkly flag keys attached to this quest; each shows its
	// live rollout state (on / partial / off) on the quest (see app/runes.go).
	Runes []string `json:"runes,omitempty"`

	// Tracks are analytics tracking events harvested from this quest's linked
	// PRs (see app/harvest.go) — each an event name plus the property keys
	// ("marks") worth analyzing. Harvest-only; never entered by hand.
	Tracks []Track `json:"tracks,omitempty"`

	// Lookouts are the per-quest usage dashboards (Amplitude/Fullstory/Hex) you
	// monitor the quest's Tracks in — captured by pasting a dashboard URL into
	// the body. A quest can hold several (adoption, funnel, retention…).
	Lookouts []Lookout `json:"lookouts,omitempty"`

	// DismissedTracks are event names the user removed (Ctrl+X on a Track), so
	// auto-harvest won't resurrect them.
	DismissedTracks []string `json:"dismissedTracks,omitempty"`

	// RuneSources maps a rune key → the "#code" of the PR whose body introduced
	// it (recorded during a find), so a rune can be aged by that PR's merge date.
	RuneSources map[string]string `json:"runeSources,omitempty"`

	// BodyLinks maps a shortened link display (left inline in the body when a
	// non-captured URL is pasted, e.g. "docs.google.com/…") → its full URL, so
	// the body stays compact while the link is still clickable/copyable.
	BodyLinks map[string]string `json:"bodyLinks,omitempty"`

	// Wards is the deprecated pre-Tracks/Lookouts field, migrated on load (see
	// store.Load) then cleared so it drops out on the next save.
	Wards []Ward `json:"wards,omitempty"`

	// ConnectionsCollapsed hides this quest's connections section in its detail
	// view (body only) — a per-quest preference.
	ConnectionsCollapsed bool `json:"connectionsCollapsed,omitempty"`

	// Deprecated worktree-pin fields, cleared on load — the agent integration
	// is now keyed on herdr workspaces, not worktrees.
	AgentWorktrees []string `json:"agentWorktrees,omitempty"`
	AgentWorktree  string   `json:"agentWorktree,omitempty"`

	// Legacy single-link fields, kept only so pre-slice data migrates on load
	// (see store.Load); cleared there so they drop out on the next save.
	JiraCode string `json:"jiraCode,omitempty"` // deprecated: migrated into JiraCodes
	PRCode   string `json:"prCode,omitempty"`   // deprecated: migrated into PRs
	PRRepo   string `json:"prRepo,omitempty"`   // deprecated: migrated into PRs
}

// RecurUnit is the cadence unit of a rite.
type RecurUnit string

const (
	RecurDaily  RecurUnit = "day"
	RecurWeekly RecurUnit = "week"
)

// Recurrence makes a quest a "rite": it recurs on a cadence instead of being a
// one-off. Every is the unit; Interval is how many units between occurrences (0
// means 1). For a weekly rite, Weekdays optionally pins which days fire (empty =
// the same weekday as the current Muster). nil Recurrence = a one-off quest.
type Recurrence struct {
	Every    RecurUnit      `json:"every"`
	Interval int            `json:"interval,omitempty"`
	Weekdays []time.Weekday `json:"weekdays,omitempty"`
}

// IsRite reports whether the quest recurs.
func (q *Quest) IsRite() bool { return q.Recurrence != nil }

// Called reports whether the quest is "called" to Camp as of day — its Muster is
// on or before day (an overdue muster still counts, rolling forward). False when
// it has no Muster.
func (q *Quest) Called(day time.Time) bool {
	return q.Muster != nil && !DateOnly(*q.Muster).After(DateOnly(day))
}

// DateOnly reduces t to its calendar date (the Y/M/D as seen in t's own
// location) anchored at UTC midnight — a timezone-independent "civil date". This
// lets a muster stored in any zone compare correctly against "now": both collapse
// to the same civil date, so a muster dated today never reads as overdue merely
// because it was written at UTC midnight while now is in a behind-UTC zone.
func DateOnly(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
}

// EffectiveInterval returns the interval with its default applied (minimum 1).
func (r *Recurrence) EffectiveInterval() int {
	if r.Interval < 1 {
		return 1
	}
	return r.Interval
}

// Next returns the first occurrence strictly after `from`, date-only. A weekly
// rite with Weekdays set returns the next listed weekday after `from`; otherwise
// it steps forward by Interval days (daily) or weeks (weekly).
func (r *Recurrence) Next(from time.Time) time.Time {
	d := DateOnly(from)
	if r.Every == RecurWeekly {
		if len(r.Weekdays) > 0 {
			want := map[time.Weekday]bool{}
			for _, w := range r.Weekdays {
				want[w] = true
			}
			for i := 1; i <= 7; i++ {
				if nd := d.AddDate(0, 0, i); want[nd.Weekday()] {
					return nd
				}
			}
			return d.AddDate(0, 0, 7) // unreachable when Weekdays is non-empty
		}
		return d.AddDate(0, 0, 7*r.EffectiveInterval())
	}
	return d.AddDate(0, 0, r.EffectiveInterval())
}

// PRLink is one linked GitHub pull request: its short code ("#47477") and the
// "owner/repo" it lives in.
type PRLink struct {
	Code string `json:"code"`
	Repo string `json:"repo"`
}

// Track is a harvested analytics tracking event: the event name exactly as it
// appears in the analytics schema (e.g. "Practice - Checkout - Navigation
// Changed") and the property keys ("marks") worth breaking usage down by.
type Track struct {
	Event string   `json:"event"`
	Marks []string `json:"marks,omitempty"`
	// SourcePR is the "#code" of the PR whose diff introduced this event — used
	// to color the track by production state (merged = live). Empty for tracks
	// found before provenance was recorded.
	SourcePR string `json:"sourcePR,omitempty"`
}

// Lookout is one per-quest usage dashboard: the URL to "scry" (open), the tool
// it lives in (amplitude | fullstory | hex, inferred from the host and used to
// tailor the incantation), and an optional custom label.
type Lookout struct {
	URL   string `json:"url"`
	Tool  string `json:"tool,omitempty"`
	Label string `json:"label,omitempty"`
	// AddedAt is when the dashboard was captured — used to show its age (a
	// dashboard has no PR, so it ages from creation).
	AddedAt time.Time `json:"addedAt,omitempty"`
}

// Ward is the deprecated combined event+dashboard type, kept only so pre-split
// data migrates on load (store.Load) into a Track (+ a Lookout when it carried
// a dashboard URL).
type Ward struct {
	Event     string   `json:"event"`
	Marks     []string `json:"marks,omitempty"`
	Dashboard string   `json:"dashboard,omitempty"`
	Tool      string   `json:"tool,omitempty"`
}

// InQuestboard reports whether a quest is currently an untriaged notice on
// the Questboard — no campaign yet, and not deliberately vaulted either.
// Questboard quests are listing-only: no active/done/canceled status
// applies until they're picked up (moved to a campaign).
func (q *Quest) InQuestboard() bool {
	return q.ProjectID == "" && q.BannerID == "" && !q.Vaulted
}

// ObjectiveProgress returns (done, total) counting only lines that classify
// as BodyObjective, skipping headings/text.
func (q *Quest) ObjectiveProgress() (done, total int) {
	for _, l := range q.Body {
		kind, _ := ClassifyBodyLine(l.Text)
		if kind != BodyObjective {
			continue
		}
		total++
		if l.Done {
			done++
		}
	}
	return done, total
}

// NextObjective returns the display text of the first not-done objective — the
// quest's "next action" — skipping headings, plain text, and completed
// objectives. ok is false when the quest has no pending objective.
func (q *Quest) NextObjective() (text string, ok bool) {
	for _, l := range q.Body {
		kind, display := ClassifyBodyLine(l.Text)
		if kind != BodyObjective || l.Done {
			continue
		}
		return display, true
	}
	return "", false
}
