package model

import "time"

type Project struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Icon       string     `json:"icon,omitempty"`
	Archived   bool       `json:"archived"`
	ArchivedAt *time.Time `json:"archivedAt,omitempty"` // when archived; places it in the Vault's day timeline
	Body       []BodyLine `json:"body"`

	// BannerID is the Banner (Area) this campaign belongs to; "" = ungrouped.
	BannerID string `json:"bannerId,omitempty"`
	// CompletedAt marks a finished campaign; drives its move to the Vault and
	// distinguishes a done project from an ongoing one. Nil = still in progress.
	CompletedAt *time.Time `json:"completedAt,omitempty"`
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
