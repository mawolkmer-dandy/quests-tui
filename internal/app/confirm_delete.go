package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// The confirm-delete dialog (ModalConfirmDelete) is the single, consistent
// "are you sure?" for every destructive action, replacing the old inline y/n
// prompt. openDeleteModal computes entity-specific copy (with live counts of
// what else is affected); executeDelete performs the deletion on confirm.

// confirmBtnGap is the spacing between the Cancel and delete buttons.
const confirmBtnGap = 3

// openDeleteModal pushes the confirmation dialog for deleting whatever target
// points at — a banner, campaign, quest, rune, lookout, or track. Returns false
// (no dialog) when the target isn't a deletable thing.
func (m *Model) openDeleteModal(target cursorTarget) bool {
	m.commitEdit() // persist any in-flight rename before the dialog takes over
	title, body, verb := "", "", "Delete"
	switch target.kind {
	case ui.RowBanner:
		b := m.findBanner(target.bannerID)
		if b == nil { // synthetic "Unassigned" isn't real
			return false
		}
		title = deleteTitle("Delete", "banner", b.Name)
		if n := m.bannerContentCount(target.bannerID); n > 0 {
			body = fmt.Sprintf("Its %s will move to Unassigned — not deleted.", plural(n, "item"))
		} else {
			body = "This banner is empty."
		}
	case ui.RowProject:
		p := m.findProject(target.projectID)
		if p == nil {
			return false
		}
		title = deleteTitle("Delete", "campaign", p.Name)
		if n := m.projectQuestCount(target.projectID); n > 0 {
			body = fmt.Sprintf("It contains %s that will be deleted too.", plural(n, "quest"))
		}
	case ui.RowQuest:
		q := m.findQuest(target.questID)
		if q == nil {
			return false
		}
		title = deleteTitle("Delete", "quest", q.Title)
		if questHasDetails(q) {
			body = "Its notes will be deleted too."
		}
	case ui.RowRune:
		title, verb = deleteTitle("Stop watching", "rune", target.runeKey), "Stop watching"
	case ui.RowLookout:
		title, verb = "Remove this lookout?", "Remove"
	case ui.RowTrack:
		title, verb = deleteTitle("Dismiss", "track", target.trackEvent), "Dismiss"
	default:
		return false
	}
	m.pushModal(&Modal{
		Kind:         ModalConfirmDelete,
		DeleteTarget: target,
		Title:        title,
		Body:         body,
		DeleteVerb:   verb,
		DeleteFocus:  1, // the delete button is focused so Enter confirms
	})
	return true
}

// deleteTitle builds the dialog title, naming the thing being deleted — e.g.
// `Delete quest "Fix the parser"?` — or falling back to "Delete this quest?"
// when it has no name yet. Long names are clipped.
func deleteTitle(verb, noun, name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return fmt.Sprintf("%s %s %q?", verb, noun, clipLabel(name, 40))
	}
	return fmt.Sprintf("%s this %s?", verb, noun)
}

// bannerContentCount is how many top-level things a banner groups — its
// campaigns plus its loose quests (quests filed directly under it, not inside a
// campaign). Quests inside a campaign follow their campaign, so they aren't
// counted separately.
func (m *Model) bannerContentCount(id string) int {
	n := 0
	for i := range m.store.Projects {
		if m.store.Projects[i].BannerID == id {
			n++
		}
	}
	for i := range m.store.Quests {
		if q := &m.store.Quests[i]; q.BannerID == id && q.ProjectID == "" {
			n++
		}
	}
	return n
}

// executeDelete performs the confirmed deletion, closes the dialog, and flashes
// a status-line toast naming what went. Every path stays undoable (Ctrl+Z) via
// the store's snapshot on save.
func (m *Model) executeDelete() tea.Cmd {
	mod := m.modal
	if mod == nil || mod.Kind != ModalConfirmDelete {
		return nil
	}
	t := mod.DeleteTarget
	toast := m.deletedToast(t) // capture the name before the entity is gone
	m.closeModal()
	switch t.kind {
	case ui.RowBanner:
		// Banners live in the hall (not a pane row); ensureHallCursor relocates
		// the now-stale selection on the next render.
		m.deleteBannerByID(t.bannerID)
	case ui.RowProject:
		m.removeCurrentRow(func() { m.deleteProjectByID(t.projectID) })
	case ui.RowQuest:
		m.removeCurrentRow(func() { m.deleteQuestByID(t.questID) })
	case ui.RowRune:
		m.removeCurrentRow(func() { m.detachRuneFromQuest(t.questID, t.runeKey) })
	case ui.RowLookout:
		m.removeCurrentRow(func() { m.removeLookout(t.questID, t.lookoutURL) })
	case ui.RowTrack:
		m.removeCurrentRow(func() { m.dismissTrack(t.questID, t.trackEvent) })
	}
	return m.showClipboardToastText(toast)
}

// deletedToast is the past-tense status-line message for a just-deleted thing,
// naming it — e.g. `Deleted campaign "Migrate"`. Read before the deletion, while
// the entity is still resolvable.
func (m *Model) deletedToast(t cursorTarget) string {
	switch t.kind {
	case ui.RowBanner:
		if b := m.findBanner(t.bannerID); b != nil {
			return "Deleted banner" + toastName(b.Name)
		}
	case ui.RowProject:
		if p := m.findProject(t.projectID); p != nil {
			return "Deleted campaign" + toastName(p.Name)
		}
	case ui.RowQuest:
		if q := m.findQuest(t.questID); q != nil {
			return "Deleted quest" + toastName(q.Title)
		}
	case ui.RowRune:
		return "Stopped watching" + toastName(t.runeKey)
	case ui.RowLookout:
		return "Removed lookout"
	case ui.RowTrack:
		return "Dismissed track" + toastName(t.trackEvent)
	}
	return "Deleted"
}

// toastName renders a clipped, quoted name to append after the verb — or ""
// (nothing) when the thing has no name yet.
func toastName(name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return fmt.Sprintf(" %q", clipLabel(name, 30))
	}
	return ""
}

// updateConfirmDelete handles the dialog's keys: Esc/n cancel, y/d confirm,
// ←/→/Tab toggle the focused button, Enter activates it.
func (m *Model) updateConfirmDelete(msg tea.KeyPressMsg) tea.Cmd {
	mod := m.modal
	switch {
	case msg.Code == tea.KeyEsc, msg.String() == "n":
		m.closeModal()
	case msg.String() == "y", msg.String() == "d":
		return m.executeDelete()
	case msg.Code == tea.KeyLeft, msg.Code == tea.KeyRight, msg.Code == tea.KeyTab:
		mod.DeleteFocus ^= 1
	case msg.Code == tea.KeyEnter:
		if mod.DeleteFocus == 1 {
			return m.executeDelete()
		}
		m.closeModal()
	}
	return nil
}

// handleConfirmDeleteClick resolves a left-click in the dialog: only a click on
// the delete button confirms; a click on Cancel, the backdrop, or anywhere else
// cancels — so a stray click can never delete.
func (m *Model) handleConfirmDeleteClick(msg tea.MouseClickMsg) tea.Cmd {
	mouse := msg.Mouse()
	if mouse.Button == tea.MouseLeft && mouse.Y == m.confirmBtnRow &&
		mouse.X >= m.confirmDeleteX0 && mouse.X < m.confirmDeleteX1 {
		return m.executeDelete()
	}
	m.closeModal()
	return nil
}

// confirmButtons renders the "Cancel" / "<verb>" pills, the focused one filled
// (Delete in the important/red hue). Returns the pill row (no outer padding — the
// caller right-aligns it) plus each pill's display width for click mapping.
func confirmButtons(verb string, focus int) (core string, cancelW, deleteW int) {
	cancel := "  Cancel  "
	del := "  " + verb + "  "
	cancelStyle := lipgloss.NewStyle().Foreground(ui.ColorSelected).Reverse(true)
	delStyle := lipgloss.NewStyle().Foreground(ui.ColorImportant)
	if focus == 0 {
		cancelStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
	} else {
		delStyle = lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(ui.ColorImportant)
	}
	core = cancelStyle.Render(cancel) + strings.Repeat(" ", confirmBtnGap) + delStyle.Render(del)
	return core, lipgloss.Width(cancel), lipgloss.Width(del)
}

// plural renders "1 quest" / "3 quests" — a small count with a pluralized noun.
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
