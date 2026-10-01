package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// A saga chains campaigns into ordered chapters via Project.NextID (Ch.1 →
// Ch.2), so a long effort can split across linked campaigns instead of one
// unwieldy one. Each campaign has at most one next chapter; the previous chapter
// is derived by scanning for whichever campaign points here.

// prevChapter returns the campaign whose NextID points at id — the previous
// chapter of a saga — or nil when nothing links here.
func (m *Model) prevChapter(id string) *model.Project {
	if id == "" {
		return nil
	}
	for i := range m.store.Projects {
		if m.store.Projects[i].NextID == id {
			return &m.store.Projects[i]
		}
	}
	return nil
}

// chapterReaches reports whether following NextID links from fromID eventually
// arrives at targetID — used to reject a link that would form a cycle. Bounded
// by the project count so a pre-existing cycle can't loop forever.
func (m *Model) chapterReaches(fromID, targetID string) bool {
	id := fromID
	for n := 0; id != "" && n <= len(m.store.Projects); n++ {
		if id == targetID {
			return true
		}
		p := m.findProject(id)
		if p == nil {
			return false
		}
		id = p.NextID
	}
	return false
}

// setNextChapter links p to the next chapter sel (or clears it when sel is ""),
// refusing a self-link or one that would close a cycle. Returns false (a no-op)
// when the link was rejected.
func (m *Model) setNextChapter(p *model.Project, sel string) bool {
	if p == nil || sel == p.ID {
		return false
	}
	if sel != "" && m.chapterReaches(sel, p.ID) {
		return false // sel already leads back to p — linking would cycle
	}
	p.NextID = sel
	m.save()
	return true
}

// openSagaPicker opens the next-chapter picker for the campaign currently shown
// in the pane (a RowProject hall selection) — a no-op when no campaign is in
// view (a room or banner is selected).
func (m *Model) openSagaPicker() tea.Cmd {
	if m.hallCursor.kind != ui.RowProject {
		return nil
	}
	m.commitEdit()
	m.pushModal(m.sagaPickerModal(m.hallCursor.projectID))
	return nil
}
