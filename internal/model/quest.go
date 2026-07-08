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
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Type        QuestType   `json:"type"`
	Status      QuestStatus `json:"status"`
	Vaulted     bool        `json:"vaulted"`
	Priority    Priority    `json:"priority,omitempty"`  // optional emphasis, shown with a left arrow; orthogonal to type/status
	Important   bool        `json:"important,omitempty"` // deprecated: migrated to Priority=High on load
	ProjectID   string      `json:"projectId"`
	Body        []BodyLine  `json:"body"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	CompletedAt *time.Time  `json:"completedAt,omitempty"`

	// Integration links, captured from URLs pasted into the body (see
	// internal/model/links.go and internal/app/links.go). JiraCode holds only
	// the first Jira issue found; PRs holds every linked GitHub PR in the
	// order it was captured. All omitempty, so existing data needs no
	// migration.
	JiraCode string   `json:"jiraCode,omitempty"` // e.g. "EPDCHAIR-5713"
	PRs      []PRLink `json:"prs,omitempty"`      // every linked GitHub PR

	// Legacy single-PR fields, kept only so pre-PRs data migrates on load (see
	// store.Load); cleared there so they drop out on the next save.
	PRCode string `json:"prCode,omitempty"` // deprecated: migrated into PRs
	PRRepo string `json:"prRepo,omitempty"` // deprecated: migrated into PRs
}

// PRLink is one linked GitHub pull request: its short code ("#47477") and the
// "owner/repo" it lives in.
type PRLink struct {
	Code string `json:"code"`
	Repo string `json:"repo"`
}

// InQuestboard reports whether a quest is currently an untriaged notice on
// the Questboard — no campaign yet, and not deliberately vaulted either.
// Questboard quests are listing-only: no active/done/canceled status
// applies until they're picked up (moved to a campaign).
func (q *Quest) InQuestboard() bool {
	return q.ProjectID == "" && !q.Vaulted
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
