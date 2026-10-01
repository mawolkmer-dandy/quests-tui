package app

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// A muster (a quest's "when" date) and a rite (recurrence) are set from one
// scheduling picker (F3 on a quest). Both live in this file: the option list the
// picker shows, and applySchedule which writes the chosen muster/rite onto the
// quest. Setting a rite also seeds its first muster so it surfaces in Camp.

// scheduleGroup captions the picker's two sections.
const (
	scheduleGroupMuster = "MUSTER"
	scheduleGroupRite   = "RITE (recurring)"
)

// scheduleOption is one selectable row in the scheduling picker. group is the
// caption rendered above the first option of each section ("" = no caption, used
// for the trailing Clear).
type scheduleOption struct {
	id    string
	label string
	group string
}

// scheduleOptions is the ordered picker list, computed from `now` so the weekly
// rite names the day it will fall on.
func scheduleOptions(now time.Time) []scheduleOption {
	weekday := now.Weekday().String()[:3]
	return []scheduleOption{
		{id: "m:today", label: "Today", group: scheduleGroupMuster},
		{id: "m:tomorrow", label: "Tomorrow"},
		{id: "m:nextmon", label: "Next Monday"},
		{id: "m:week", label: "In a week"},
		{id: "r:daily", label: "Every day", group: scheduleGroupRite},
		{id: "r:weekdays", label: "Every weekday"},
		{id: "r:weekly", label: "Every week (" + weekday + ")"},
		{id: "r:biweekly", label: "Every 2 weeks"},
		{id: "clear", label: "Clear schedule"},
	}
}

// openSchedulePicker opens the muster/rite picker for the quest under the cursor
// (a list row). The commit is done here since the caller is mid-edit on a row.
func (m *Model) openSchedulePicker() tea.Cmd {
	if m.cursor.kind != ui.RowQuest {
		return nil
	}
	q := m.findQuest(m.cursor.questID)
	if q == nil {
		return nil
	}
	m.commitEdit()
	m.openScheduleFor(q)
	return nil
}

// openScheduleFor stacks the muster/rite picker for a specific quest — shared by
// the list path (openSchedulePicker) and the quest detail page, so scheduling
// works from inside a quest, not only from a list.
func (m *Model) openScheduleFor(q *model.Quest) {
	now := time.Now()
	m.pushModal(&Modal{
		Kind:          ModalSchedulePicker,
		TargetQuestID: q.ID,
		Title:         q.Title,
		Body:          scheduleSummary(q, now),
		PickerItems:   scheduleItems(now),
	})
}

// scheduleItems adapts scheduleOptions into the generic pickerItem list the
// modal machinery navigates (ID carries the option id; the render reads groups
// back from scheduleOptions).
func scheduleItems(now time.Time) []pickerItem {
	opts := scheduleOptions(now)
	items := make([]pickerItem, len(opts))
	for i, o := range opts {
		items[i] = pickerItem{ID: o.id, Label: o.label}
	}
	return items
}

// applySchedule writes the chosen muster/rite onto the target quest and returns
// a status-line toast describing it. Unknown ids are a no-op.
func (m *Model) applySchedule(questID, optID string) tea.Cmd {
	q := m.findQuest(questID)
	if q == nil {
		return nil
	}
	now := time.Now()
	today := model.DateOnly(now)
	set := func(muster *time.Time, rec *model.Recurrence) {
		q.Muster, q.Recurrence = muster, rec
		q.UpdatedAt = now
		m.save()
	}
	at := func(d time.Time) *time.Time { dd := model.DateOnly(d); return &dd }

	switch optID {
	case "clear":
		set(nil, nil)
		return m.showClipboardToastText("schedule cleared")
	case "m:today":
		set(at(today), nil)
	case "m:tomorrow":
		set(at(today.AddDate(0, 0, 1)), nil)
	case "m:nextmon":
		set(at(nextWeekday(today, time.Monday)), nil)
	case "m:week":
		set(at(today.AddDate(0, 0, 7)), nil)
	case "r:daily":
		rec := &model.Recurrence{Every: model.RecurDaily, Interval: 1}
		set(at(firstMuster(rec, today)), rec)
	case "r:weekdays":
		rec := &model.Recurrence{Every: model.RecurWeekly, Interval: 1, Weekdays: weekdaysMonFri()}
		set(at(firstMuster(rec, today)), rec)
	case "r:weekly":
		rec := &model.Recurrence{Every: model.RecurWeekly, Interval: 1}
		set(at(today), rec) // anchored to today's weekday
	case "r:biweekly":
		rec := &model.Recurrence{Every: model.RecurWeekly, Interval: 2}
		set(at(today), rec)
	default:
		return nil
	}
	return m.showClipboardToastText(scheduleSummary(q, now))
}

// recycleRite advances a rite to its next occurrence instead of finishing it:
// its muster rolls forward (from the later of today and its current muster, so
// an overdue rite doesn't try to catch up), it returns to open — leaving Camp
// until that day — and never lands in the done/Vault state. Returns false for a
// non-rite, so the caller completes it normally. Does not save; the caller does.
func recycleRite(q *model.Quest) bool {
	if !q.IsRite() {
		return false
	}
	base := model.DateOnly(time.Now())
	if q.Muster != nil && model.DateOnly(*q.Muster).After(base) {
		base = model.DateOnly(*q.Muster)
	}
	next := q.Recurrence.Next(base)
	q.Muster = &next
	q.Status = model.StatusOpen
	q.CompletedAt = nil
	return true
}

// firstMuster is the first occurrence of a rite on or after `today`. A daily or
// plain weekly/biweekly rite starts today; a weekday-pinned rite starts on the
// next listed weekday (today if today qualifies).
func firstMuster(rec *model.Recurrence, today time.Time) time.Time {
	today = model.DateOnly(today)
	if len(rec.Weekdays) == 0 {
		return today
	}
	for _, w := range rec.Weekdays {
		if today.Weekday() == w {
			return today
		}
	}
	return rec.Next(today) // next listed weekday after today
}

// scheduleSummary is a short human description of a quest's current schedule for
// the picker subtitle and the set-confirmation toast (reusing the same label
// atoms the row badge shows).
func scheduleSummary(q *model.Quest, now time.Time) string {
	if q.Recurrence != nil {
		s := ui.RiteCadence(q.Recurrence) + " rite"
		if q.Muster != nil {
			when, _ := ui.MusterWhen(*q.Muster, now)
			s += " · next " + when
		}
		return s
	}
	if q.Muster != nil {
		when, _ := ui.MusterWhen(*q.Muster, now)
		return "musters " + when
	}
	return "not scheduled"
}

// nextWeekday returns the next date strictly after `from` that falls on w.
func nextWeekday(from time.Time, w time.Weekday) time.Time {
	d := model.DateOnly(from)
	for i := 1; i <= 7; i++ {
		if nd := d.AddDate(0, 0, i); nd.Weekday() == w {
			return nd
		}
	}
	return d.AddDate(0, 0, 7)
}

func weekdaysMonFri() []time.Weekday {
	return []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
}
