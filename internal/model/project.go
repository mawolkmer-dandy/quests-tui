package model

type Project struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Icon     string     `json:"icon,omitempty"`
	Archived bool       `json:"archived"`
	Body     []BodyLine `json:"body"`
}

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
