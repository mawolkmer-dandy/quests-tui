package model

import "time"

type Project struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Icon       string     `json:"icon,omitempty"`
	Archived   bool       `json:"archived"`
	ArchivedAt *time.Time `json:"archivedAt,omitempty"` // when archived; places it in the Vault's day timeline
	Body       []BodyLine `json:"body"`
	// BodyLinks maps a shortened link display left inline in the notes to the
	// full URL it stands for — so a pasted campaign-document link shows compact
	// but the real address is preserved (mirrors Quest.BodyLinks).
	BodyLinks map[string]string `json:"bodyLinks,omitempty"`

	// Priority is an optional emphasis on a campaign, shown with a left arrow —
	// the same marker quests use (medium/high up, low down). Purely a visual cue
	// (it doesn't reorder campaigns). Cycled with the priority key.
	Priority Priority `json:"priority,omitempty"`

	// BannerID is the Banner (Area) this campaign belongs to; "" = ungrouped.
	BannerID string `json:"bannerId,omitempty"`
	// CompletedAt marks a finished campaign; drives its move to the Vault and
	// distinguishes a done project from an ongoing one. Nil = still in progress.
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	// NextID chains this campaign to the next chapter of a saga (Ch.1 → Ch.2),
	// so a long effort can split across linked campaigns instead of one giant
	// one. "" = no continuation. The previous chapter is derived by scanning for
	// whichever campaign points here (see the app's prevChapter).
	NextID string `json:"nextId,omitempty"`
}

// IsCompleted reports whether the campaign has been finished (and belongs in
// the Vault's logbook).
func (p *Project) IsCompleted() bool { return p.CompletedAt != nil }

// ProgressBucket maps a 0..1 completion ratio to one of 5 text-glyph buckets,
// mirroring Things' circular per-project progress indicator.
func ProgressBucket(done, total int) string {
	if total == 0 {
		return "○"
	}
	ratio := float64(done) / float64(total)
	switch {
	case ratio <= 0:
		return "○"
	case ratio <= 0.25:
		return "◔"
	case ratio <= 0.50:
		return "◑"
	case ratio <= 0.75:
		return "◕"
	default:
		return "●"
	}
}
