package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// Store is the entire persisted state of the app, round-tripped to a single
// local JSON file. There is no DB at this scale — a personal todo list is a
// few hundred quests at most, and a flat file stays human-readable.
type Store struct {
	Projects []model.Project `json:"projects"`
	Quests   []model.Quest   `json:"quests"`
	// WildsOrder is the user's manual ordering of quests in the Wilds view, by
	// quest ID — independent of the per-campaign order in the Tavern. Quests
	// not listed here fall in after, sorted by tier. Stale IDs are ignored.
	WildsOrder []string `json:"wildsOrder,omitempty"`
}

// DefaultDir is where Quests keeps its data file, config, and backups:
// $XDG_CONFIG_HOME/quests, or ~/.config/quests when that variable is unset
// — the conventional home for a CLI's files.
func DefaultDir() (string, error) {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "quests"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "quests"), nil
}

// DefaultPath returns the data file inside DefaultDir.
func DefaultPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.json"), nil
}

// Load reads the store from path, seeding it with example data on first
// run. If the file doesn't exist but data from an earlier location does
// (the pre-XDG ~/.quests, or the original ~/.questlog), that data is copied
// over first — the old file is left untouched.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if migrated, mErr := migrateLegacyData(path); mErr == nil && migrated {
			return Load(path)
		}
		s := seed()
		if err := Save(path, s); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}

	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	// Migrate the old boolean `important` flag to the Priority scale (High).
	// The vestigial field is cleared so it drops out on the next save.
	for i := range s.Quests {
		if s.Quests[i].Important && s.Quests[i].Priority == model.PriorityNone {
			s.Quests[i].Priority = model.PriorityHigh
		}
		s.Quests[i].Important = false

		// Migrate the old single-PR fields into the PRs slice. The vestigial
		// fields are cleared so they drop out on the next save.
		if len(s.Quests[i].PRs) == 0 && s.Quests[i].PRCode != "" {
			s.Quests[i].PRs = []model.PRLink{{Code: s.Quests[i].PRCode, Repo: s.Quests[i].PRRepo}}
		}
		s.Quests[i].PRCode = ""
		s.Quests[i].PRRepo = ""

		// Migrate the old single-Jira field into the JiraCodes slice, likewise.
		if len(s.Quests[i].JiraCodes) == 0 && s.Quests[i].JiraCode != "" {
			s.Quests[i].JiraCodes = []string{s.Quests[i].JiraCode}
		}
		s.Quests[i].JiraCode = ""

		// The agent integration moved from worktrees to herdr workspaces; drop
		// any old worktree pins (no reliable worktree→workspace mapping).
		s.Quests[i].AgentWorktrees = nil
		s.Quests[i].AgentWorktree = ""
	}
	return &s, nil
}

// migrateLegacyData copies the data file from the newest earlier location
// that still has one to path, reporting whether a copy happened. Only the
// data file moves automatically; config and backups don't follow (a manual
// move handles those on upgrade).
func migrateLegacyData(path string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	legacyPaths := []string{
		filepath.Join(home, ".quests", "data.json"),   // pre-XDG location
		filepath.Join(home, ".questlog", "data.json"), // original app name
	}
	var data []byte
	for _, legacy := range legacyPaths {
		d, err := os.ReadFile(legacy)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		data = d
		break
	}
	if data == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// Save atomically writes the store to path: write a temp file in the same
// directory, then rename over the target, so a crash mid-write can never
// corrupt the existing data file.
func Save(path string, s *Store) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".data-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// Snapshot serializes the store to JSON bytes for the undo stack. Errors are
// swallowed (a store that won't marshal can't have been loaded), returning nil.
func (s *Store) Snapshot() []byte {
	b, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	return b
}

// RestoreSnapshot rebuilds a store from Snapshot bytes.
func RestoreSnapshot(b []byte) (*Store, error) {
	var s Store
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func seed() *Store {
	now := time.Now()
	projectID := NewID()

	return &Store{
		Projects: []model.Project{
			{ID: projectID, Name: "Homestead"},
		},
		Quests: []model.Quest{
			{
				ID:        NewID(),
				Title:     "Repair the phial",
				Type:      model.QuestTypeMain,
				Status:    model.StatusOpen,
				ProjectID: projectID,
				Body: []model.BodyLine{
					{ID: NewID(), Text: "Find a replacement phial and get the still working again."},
					{ID: NewID(), Text: "# Prep"},
					{ID: NewID(), Text: "- Find a glassblower", Done: true},
					{ID: NewID(), Text: "- Source replacement phial"},
					{ID: NewID(), Text: "- Refit the still"},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
}
