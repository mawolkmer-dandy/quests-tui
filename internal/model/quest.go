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
	Important   bool        `json:"important"` // flagged as priority work; shown with a left arrow, orthogonal to type/status
	ProjectID   string      `json:"projectId"`
	Body        []BodyLine  `json:"body"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	CompletedAt *time.Time  `json:"completedAt,omitempty"`
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
