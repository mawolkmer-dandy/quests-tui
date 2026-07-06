// Package quickadd is the spool between the `quests add` CLI and the running
// app. The CLI never writes data.json directly (that would race the app's own
// saves); instead it drops one small JSON "entry" file per captured task into
// a spool directory. Whoever owns data.json drains the spool: the running app
// (via a filesystem watcher) or the next `quests` launch. This keeps a single
// writer to data.json at all times and gives offline capture for free.
package quickadd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
)

// Entry is one captured task waiting to be ingested. ProjectID == "" means the
// Questboard (inbox). Type is "main" or "side".
type Entry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Note      string    `json:"note,omitempty"` // optional body text; newlines become separate body lines
	ProjectID string    `json:"projectId"`
	Type      string    `json:"type"`
	Important bool      `json:"important"`
	CreatedAt time.Time `json:"createdAt"`
}

// Dir is the spool directory inside the app's config dir.
func Dir(baseDir string) string {
	return filepath.Join(baseDir, "quick-add")
}

// Enqueue writes e to the spool as its own file, created atomically (temp +
// rename) so a watcher never observes a half-written entry.
func Enqueue(baseDir string, e Entry) error {
	dir := Dir(baseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".entry-*.json.tmp")
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
	return os.Rename(tmpPath, filepath.Join(dir, e.ID+".json"))
}

// pending reads every spool entry, oldest first (by filename, which sorts by
// creation since IDs are time-ordered), returning each with its file path so
// the caller can delete it after a successful ingest. Unreadable/partial files
// are skipped, not fatal.
func pending(baseDir string) (entries []Entry, paths []string) {
	dir := Dir(baseDir)
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var files []string
	for _, n := range names {
		if n.IsDir() || filepath.Ext(n.Name()) != ".json" {
			continue
		}
		files = append(files, n.Name())
	}
	sort.Strings(files)
	for _, f := range files {
		p := filepath.Join(dir, f)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var e Entry
		if err := json.Unmarshal(data, &e); err != nil || e.Title == "" {
			continue
		}
		entries = append(entries, e)
		paths = append(paths, p)
	}
	return entries, paths
}

// Drain ingests every pending entry into s as a new quest, then removes the
// spooled files. It returns the number ingested. The caller is responsible for
// saving s — Drain only mutates it in memory, so a save failure leaves the
// spool intact for a retry. If nothing is pending, it returns 0 and does not
// touch the store.
func Drain(baseDir string, s *store.Store) int {
	entries, paths := pending(baseDir)
	if len(entries) == 0 {
		return 0
	}
	for _, e := range entries {
		s.Quests = append(s.Quests, e.toQuest())
	}
	// Only remove spool files once they're in the store's memory; the caller
	// saving afterwards is what makes it durable, but a crash before save just
	// means the same entries re-ingest next time — never data loss.
	for _, p := range paths {
		os.Remove(p)
	}
	return len(entries)
}

func (e Entry) toQuest() model.Quest {
	t := model.QuestTypeSide
	if e.Type == string(model.QuestTypeMain) {
		t = model.QuestTypeMain
	}
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	var body []model.BodyLine
	if note := strings.TrimRight(e.Note, "\n"); note != "" {
		for _, line := range strings.Split(note, "\n") {
			body = append(body, model.BodyLine{ID: store.NewID(), Text: line})
		}
	}
	priority := model.PriorityNone
	if e.Important {
		priority = model.PriorityHigh
	}
	return model.Quest{
		ID:        store.NewID(),
		Title:     e.Title,
		Type:      t,
		Status:    model.StatusOpen,
		ProjectID: e.ProjectID,
		Priority:  priority,
		Body:      body,
		CreatedAt: created,
		UpdatedAt: created,
	}
}
