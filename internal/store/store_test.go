package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
)

// TestLoadMigratesLegacyPR verifies the pre-PRs single-PR fields migrate into
// the PRs slice on load, and the vestigial fields are cleared so they drop out
// on the next save.
func TestLoadMigratesLegacyPR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	legacy := Store{
		Quests: []model.Quest{
			{ID: "a", PRCode: "#47477", PRRepo: "orthly/orthlyweb"},
			{ID: "b"}, // no PR
			{ID: "c", PRs: []model.PRLink{{Code: "#1", Repo: "x/y"}}, PRCode: "#99", PRRepo: "x/y"}, // already migrated: keep PRs, drop legacy
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	qa := s.Quests[0]
	if len(qa.PRs) != 1 || qa.PRs[0].Code != "#47477" || qa.PRs[0].Repo != "orthly/orthlyweb" {
		t.Errorf("quest a PRs = %+v, want single #47477", qa.PRs)
	}
	if qa.PRCode != "" || qa.PRRepo != "" {
		t.Errorf("quest a legacy fields not cleared: %q %q", qa.PRCode, qa.PRRepo)
	}

	if len(s.Quests[1].PRs) != 0 {
		t.Errorf("quest b PRs = %+v, want none", s.Quests[1].PRs)
	}

	qc := s.Quests[2]
	if len(qc.PRs) != 1 || qc.PRs[0].Code != "#1" {
		t.Errorf("quest c PRs = %+v, want the pre-existing #1 (no clobber)", qc.PRs)
	}
	if qc.PRCode != "" || qc.PRRepo != "" {
		t.Errorf("quest c legacy fields not cleared: %q %q", qc.PRCode, qc.PRRepo)
	}
}

// TestLoadMigratesLegacyJira verifies the pre-slice single-Jira field migrates
// into JiraCodes on load and is cleared, without clobbering existing JiraCodes.
func TestLoadMigratesLegacyJira(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	legacy := Store{
		Quests: []model.Quest{
			{ID: "a", JiraCode: "EPDCHAIR-1"},
			{ID: "b"}, // no Jira
			{ID: "c", JiraCodes: []string{"ES-2"}, JiraCode: "ES-9"}, // already migrated: keep, drop legacy
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if qa := s.Quests[0]; len(qa.JiraCodes) != 1 || qa.JiraCodes[0] != "EPDCHAIR-1" || qa.JiraCode != "" {
		t.Errorf("quest a = %+v, want JiraCodes [EPDCHAIR-1], legacy cleared", qa)
	}
	if len(s.Quests[1].JiraCodes) != 0 {
		t.Errorf("quest b JiraCodes = %+v, want none", s.Quests[1].JiraCodes)
	}
	if qc := s.Quests[2]; len(qc.JiraCodes) != 1 || qc.JiraCodes[0] != "ES-2" || qc.JiraCode != "" {
		t.Errorf("quest c = %+v, want pre-existing [ES-2] kept, legacy cleared", qc)
	}
}
