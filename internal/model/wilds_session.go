package model

import "time"

// WildsSession is one focus session out in the Wilds: the quest ventured, when
// the session started and ended, and whether it ended in the quest being
// completed (vs a plain "make camp" bail). It is the timestamped record the
// "focused today" stat is drawn from, and — later — the Vault timeline.
type WildsSession struct {
	QuestID   string    `json:"questId"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Completed bool      `json:"completed,omitempty"`
}

// Duration is how long the session lasted.
func (s WildsSession) Duration() time.Duration { return s.End.Sub(s.Start) }
