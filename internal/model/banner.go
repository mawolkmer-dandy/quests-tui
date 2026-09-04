package model

// Banner is an Area: a persistent sphere (a team, a domain, "Home") that groups
// campaigns and can hold ongoing loose quests directly. Unlike a Campaign a
// Banner never "completes" — it's the standing container work flows through.
// A campaign with an empty BannerID is ungrouped (shown at the top of the hall).
type Banner struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon,omitempty"`
}
