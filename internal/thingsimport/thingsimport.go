// Package thingsimport converts a local Things 3 database into Quests
// campaigns and quests. Things is Apple-only and its data lives in a
// sandboxed group container, so the import shells out to the system
// `sqlite3` (always present on macOS) against a private copy of the DB —
// it never touches the live database.
//
// Mapping:
//   - Things Area           → Campaign
//   - Things Project        → a side Quest under that area's campaign; the
//     project's notes become the quest body, its
//     headings become "# " lines and its to-dos
//     "- " objectives (checked if completed)
//   - loose to-do in an area→ a side Quest under that campaign
//   - Inbox / Someday item  → a Questboard quest (no campaign)
//   - Logbook item (done or
//     cancelled)            → a Vault quest (kept done if completed)
//
// A to-do's checklist items and notes come across on its quest too.
package thingsimport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
)

// Things TMTask.type / .status / .start values.
const (
	typeTodo    = 0
	typeProject = 1
	typeHeading = 2

	statusCanceled  = 2
	statusCompleted = 3

	startSomeday = 2
)

type Options struct {
	DBPath   string // explicit path to main.sqlite; empty = auto-locate
	DataPath string // Quests data.json to merge into
	DryRun   bool   // build & summarize without writing
	Replace  bool   // start from an empty store instead of appending to existing data
}

type Result struct {
	DBPath        string
	Campaigns     int
	CampaignTasks int // quests placed under a campaign
	Questboard    int
	Vault         int
	BackupPath    string // pre-import backup of data.json (empty on dry run)
}

func (r Result) Quests() int { return r.CampaignTasks + r.Questboard + r.Vault }

type task struct {
	UUID, Title, Notes     string
	Type, Status, Start    int
	Project, Area, Heading string
	Index                  int
}

// Run performs the import and returns a summary.
func Run(opts Options) (Result, error) {
	dbPath := opts.DBPath
	if dbPath == "" {
		found, err := locateDB()
		if err != nil {
			return Result{}, err
		}
		dbPath = found
	}

	tmpDB, cleanup, err := copyForRead(dbPath)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	areas, err := queryAreas(tmpDB)
	if err != nil {
		return Result{}, err
	}
	tasks, err := queryTasks(tmpDB)
	if err != nil {
		return Result{}, err
	}
	checklist := queryChecklist(tmpDB) // best-effort; nil if the table is absent

	res := Result{DBPath: dbPath}
	var campaigns []model.Project
	var quests []model.Quest
	now := time.Now()

	// Area → new campaign ID.
	campaignID := map[string]string{}
	for _, a := range areas {
		id := store.NewID()
		campaignID[a.uuid] = id
		campaigns = append(campaigns, model.Project{ID: id, Name: a.title})
		res.Campaigns++
	}

	// project uuid → its live (non-trashed) presence, so a to-do whose parent
	// project was trashed is treated as standalone.
	isProject := map[string]bool{}
	for _, t := range tasks {
		if t.Type == typeProject {
			isProject[t.UUID] = true
		}
	}

	for _, t := range tasks {
		switch t.Type {
		case typeProject:
			q := m2quest(t, now)
			q.Body = projectBody(t, tasks, checklist)
			place(&q, t, campaignID)
			quests = append(quests, q)
			tally(&res, q)
		case typeTodo:
			if t.Project != "" && isProject[t.Project] {
				continue // consumed into its project's body
			}
			q := m2quest(t, now)
			q.Body = append(notesLines(t.Notes), checklistLines(t.UUID, checklist, 0)...)
			place(&q, t, campaignID)
			quests = append(quests, q)
			tally(&res, q)
		}
		// headings (type 2) are consumed into project bodies
	}

	if opts.DryRun {
		return res, nil
	}

	backup, err := preImportBackup(opts.DataPath)
	if err != nil {
		return Result{}, err
	}
	res.BackupPath = backup

	// --replace starts from an empty store (the previous data is safe in the
	// backup above); otherwise merge the import into whatever's already there.
	s := &store.Store{}
	if !opts.Replace {
		if s, err = store.Load(opts.DataPath); err != nil {
			return Result{}, err
		}
	}
	s.Projects = append(s.Projects, campaigns...)
	s.Quests = append(s.Quests, quests...)
	if err := store.Save(opts.DataPath, s); err != nil {
		return Result{}, err
	}
	return res, nil
}

func m2quest(t task, now time.Time) model.Quest {
	return model.Quest{
		ID:        store.NewID(),
		Title:     strings.TrimSpace(t.Title),
		Type:      model.QuestTypeSide,
		Status:    model.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// place assigns a quest's campaign / vault / done state from its Things
// location. Logbook (done/cancelled) → Vault; Someday → Questboard; in an
// area → that campaign; otherwise (Inbox / no area) → Questboard.
func place(q *model.Quest, t task, campaignID map[string]string) {
	cid := campaignID[t.Area]
	switch {
	case t.Status == statusCompleted || t.Status == statusCanceled:
		q.Vaulted = true
		q.ProjectID = cid // keep the campaign tag in the Vault, if any
		if t.Status == statusCompleted {
			q.Status = model.StatusDone
		}
	case t.Start == startSomeday:
		// Questboard: no campaign.
	case cid != "":
		q.ProjectID = cid
	}
}

func tally(res *Result, q model.Quest) {
	switch {
	case q.Vaulted:
		res.Vault++
	case q.ProjectID != "":
		res.CampaignTasks++
	default:
		res.Questboard++
	}
}

// projectBody builds a project's quest body: its notes, then its
// project-level children (loose to-dos and headings) in index order, with
// each heading's to-dos nested beneath it.
func projectBody(p task, tasks []task, checklist map[string][]checkItem) []model.BodyLine {
	body := notesLines(p.Notes)

	var top []task
	for _, t := range tasks {
		if t.Project == p.UUID && t.Heading == "" && (t.Type == typeTodo || t.Type == typeHeading) {
			top = append(top, t)
		}
	}
	sort.SliceStable(top, func(i, j int) bool { return top[i].Index < top[j].Index })

	// sep adds a blank separator line, but never two in a row or a leading
	// one — so headings end up with a blank above and below except at the
	// very start/end of the body.
	sep := func() {
		if len(body) > 0 && strings.TrimSpace(body[len(body)-1].Text) != "" {
			body = append(body, line("", false, 0))
		}
	}

	for i, c := range top {
		if c.Type == typeHeading {
			sep() // blank above the heading (unless it's the first line)
			body = append(body, line("# "+c.Title, false, 0))
			var subs []task
			for _, t := range tasks {
				if t.Heading == c.UUID && t.Type == typeTodo {
					subs = append(subs, t)
				}
			}
			sort.SliceStable(subs, func(i, j int) bool { return subs[i].Index < subs[j].Index })
			for _, s := range subs {
				body = append(body, todoLines(s, checklist, 1)...)
			}
			if i < len(top)-1 {
				sep() // blank below the heading's section (unless it's the last)
			}
			continue
		}
		body = append(body, todoLines(c, checklist, 0)...)
	}
	return body
}

// todoLines renders a to-do at the given indent, with its checklist items
// nested one level deeper.
func todoLines(t task, checklist map[string][]checkItem, indent int) []model.BodyLine {
	out := []model.BodyLine{line("- "+t.Title, t.Status == statusCompleted, indent)}
	return append(out, checklistLines(t.UUID, checklist, indent+1)...)
}

func checklistLines(taskUUID string, checklist map[string][]checkItem, indent int) []model.BodyLine {
	var out []model.BodyLine
	for _, c := range checklist[taskUUID] {
		out = append(out, line("- "+c.title, c.status == statusCompleted, indent))
	}
	return out
}

func notesLines(notes string) []model.BodyLine {
	notes = strings.TrimRight(strings.ReplaceAll(notes, "\r\n", "\n"), "\n")
	if strings.TrimSpace(notes) == "" {
		return nil
	}
	var out []model.BodyLine
	for _, l := range strings.Split(notes, "\n") {
		out = append(out, line(l, false, 0))
	}
	return out
}

func line(text string, done bool, indent int) model.BodyLine {
	return model.BodyLine{ID: store.NewID(), Text: text, Done: done, Indent: indent}
}

// --- Things DB access ---------------------------------------------------

func locateDB() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(home, "Library", "Group Containers", "JLMPQHK86H.com.culturedcode.ThingsMac")
	matches, _ := filepath.Glob(filepath.Join(base, "*", "Things Database.thingsdatabase", "main.sqlite"))
	if len(matches) == 0 {
		// Fall back to a recursive walk in case the layout differs by version.
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && d.Name() == "main.sqlite" {
				matches = append(matches, p)
			}
			return nil
		})
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("couldn't find the Things database under %s\n"+
			"Grant your terminal Full Disk Access (System Settings → Privacy & Security), "+
			"or copy main.sqlite somewhere readable and pass --things-db <path>", base)
	}
	return matches[0], nil
}

// copyForRead copies the DB (and its -wal/-shm sidecars) to a temp dir and
// returns the copy's path, so queries never touch the live database and see
// any not-yet-checkpointed WAL data.
func copyForRead(dbPath string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "quests-things-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(tmp) }

	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(dbPath + suffix)
		if err != nil {
			if suffix == "" {
				cleanup()
				if os.IsPermission(err) {
					return "", func() {}, fmt.Errorf("can't read %s: permission denied\n"+
						"Grant your terminal Full Disk Access (System Settings → Privacy & Security → Full Disk Access), "+
						"or copy the database somewhere readable and pass --things-db <path>", dbPath)
				}
				return "", func() {}, err
			}
			continue // -wal/-shm are optional
		}
		if err := os.WriteFile(filepath.Join(tmp, "main.sqlite"+suffix), data, 0o600); err != nil {
			cleanup()
			return "", func() {}, err
		}
	}
	return filepath.Join(tmp, "main.sqlite"), cleanup, nil
}

func queryAreas(dbPath string) ([]struct{ uuid, title string }, error) {
	rows, err := query(dbPath, `SELECT uuid, title FROM TMArea ORDER BY "index"`)
	if err != nil {
		return nil, err
	}
	var out []struct{ uuid, title string }
	for _, r := range rows {
		out = append(out, struct{ uuid, title string }{str(r, "uuid"), str(r, "title")})
	}
	return out, nil
}

func queryTasks(dbPath string) ([]task, error) {
	rows, err := query(dbPath, `SELECT uuid, title, type, status, notes, "start",
		project, area, heading, "index" AS idx FROM TMTask WHERE trashed=0`)
	if err != nil {
		return nil, err
	}
	var out []task
	for _, r := range rows {
		out = append(out, task{
			UUID: str(r, "uuid"), Title: str(r, "title"), Notes: str(r, "notes"),
			Type: num(r, "type"), Status: num(r, "status"), Start: num(r, "start"),
			Project: str(r, "project"), Area: str(r, "area"), Heading: str(r, "heading"),
			Index: num(r, "idx"),
		})
	}
	return out, nil
}

type checkItem struct {
	title  string
	status int
}

func queryChecklist(dbPath string) map[string][]checkItem {
	rows, err := query(dbPath, `SELECT title, task, status, "index" AS idx
		FROM TMChecklistItem ORDER BY idx`)
	if err != nil {
		return nil // table may not exist on older DBs
	}
	out := map[string][]checkItem{}
	for _, r := range rows {
		t := str(r, "task")
		out[t] = append(out[t], checkItem{title: str(r, "title"), status: num(r, "status")})
	}
	return out
}

func query(dbPath, sql string) ([]map[string]any, error) {
	cmd := exec.Command("sqlite3", "-json", dbPath, sql)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sqlite3: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	out := bytes.TrimSpace(stdout.Bytes())
	if len(out) == 0 {
		return nil, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parsing sqlite3 output: %w", err)
	}
	return rows, nil
}

func str(m map[string]any, k string) string {
	if v, ok := m[k]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprint(v)
	}
	return ""
}

func num(m map[string]any, k string) int {
	if v, ok := m[k]; ok && v != nil {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return 0
}

// preImportBackup copies data.json to backups/preimport-<timestamp>.json
// before the import mutates it. Returns "" if there's nothing to back up.
func preImportBackup(dataPath string) (string, error) {
	data, err := os.ReadFile(dataPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	dir := filepath.Join(filepath.Dir(dataPath), "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, "preimport-"+time.Now().Format("2006-01-02-150405")+".json")
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", err
	}
	return dest, nil
}
