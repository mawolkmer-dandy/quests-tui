package app

import "github.com/mawolkmer-dandy/quests-tui/internal/model"

// openQuestDetailForTest sets up the quest focus modal plus the shared body
// editor the way openQuestDetail does at runtime — for tests that build a Model
// by hand (no pushModal).
func (m *Model) openQuestDetailForTest(q *model.Quest) {
	m.modal = &Modal{Kind: ModalQuestDetail, QuestID: q.ID}
	m.seedBody(ownerQuest, q.ID)
}
