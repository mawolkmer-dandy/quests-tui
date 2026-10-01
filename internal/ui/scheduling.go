package ui

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// Presentation atoms for a quest's schedule (its muster date + rite cadence),
// shared by the row badge here and the scheduling picker's subtitle/toast in the
// app package, so both describe a schedule the same way.

// RiteCadence names a recurrence: "daily", "weekday", "weekly", "biweekly", or
// "every N days/weeks".
func RiteCadence(r *model.Recurrence) string {
	switch {
	case r.Every == model.RecurDaily && r.EffectiveInterval() == 1:
		return "daily"
	case r.Every == model.RecurWeekly && len(r.Weekdays) == 5:
		return "weekday"
	case r.Every == model.RecurWeekly && r.EffectiveInterval() == 1:
		return "weekly"
	case r.Every == model.RecurWeekly && r.EffectiveInterval() == 2:
		return "biweekly"
	case r.Every == model.RecurDaily:
		return "every " + strconv.Itoa(r.EffectiveInterval()) + " days"
	default:
		return "every " + strconv.Itoa(r.EffectiveInterval()) + " weeks"
	}
}

// MusterWhen renders a muster date relative to now — "today", "tomorrow",
// "overdue", or a weekday+date ("Tue Jan 2") — and reports whether it's overdue.
func MusterWhen(muster, now time.Time) (label string, overdue bool) {
	d, today := model.DateOnly(muster), model.DateOnly(now)
	switch {
	case d.Before(today):
		return "overdue", true
	case d.Equal(today):
		return "today", false
	case d.Equal(today.AddDate(0, 0, 1)):
		return "tomorrow", false
	default:
		return MusterDate(muster), false
	}
}

// MusterDate is a muster's concrete calendar date ("Tue Jan 2") — used when a
// relative word ("overdue"/"today") doesn't fit, e.g. a finished task whose
// muster is in the past isn't "overdue", it just happened on that date.
func MusterDate(muster time.Time) string {
	d := model.DateOnly(muster)
	return d.Weekday().String()[:3] + " " + d.Format("Jan 2")
}

// QuestScheduleBadge is the terse muster/rite marker shown after a quest's title:
// "↻ daily" for a rite plus its next muster ("today" / "Tue Jan 2"), with an
// overdue muster tinted. Empty when the quest has no schedule.
func QuestScheduleBadge(q *model.Quest, now time.Time) string {
	if q.Recurrence == nil && q.Muster == nil {
		return ""
	}
	out := ""
	if q.Recurrence != nil {
		out += StyleMuted.Render(" ↻ " + RiteCadence(q.Recurrence))
	}
	if q.Muster != nil {
		when, overdue := MusterWhen(*q.Muster, now)
		style := StyleMuted
		if q.Status == model.StatusDone {
			// A finished task isn't overdue — show the date it was set for, muted.
			when = MusterDate(*q.Muster)
		} else if overdue {
			style = lipgloss.NewStyle().Foreground(ColorImportant)
		}
		out += style.Render(" · " + when)
	}
	return out
}

// QuestScheduleMeta is the schedule value for the quest-detail metadata row:
// "↻ daily", the next muster ("today"/"Tue Jan 2"/"overdue", overdue tinted), or
// both joined "↻ daily · today". Empty when the quest has no schedule, so the
// metadata row is omitted entirely.
func QuestScheduleMeta(q *model.Quest, now time.Time) string {
	if q.Recurrence == nil && q.Muster == nil {
		return ""
	}
	var parts []string
	if q.Recurrence != nil {
		parts = append(parts, "↻ "+RiteCadence(q.Recurrence))
	}
	if q.Muster != nil {
		when, overdue := MusterWhen(*q.Muster, now)
		if q.Status == model.StatusDone {
			when = MusterDate(*q.Muster) // finished: its date, not "overdue"
		} else if overdue {
			when = lipgloss.NewStyle().Foreground(ColorImportant).Render(when)
		}
		parts = append(parts, when)
	}
	return strings.Join(parts, " · ")
}
