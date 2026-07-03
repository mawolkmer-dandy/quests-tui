package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupsDir returns the backups folder inside DefaultDir.
func BackupsDir() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "backups"), nil
}

// LatestBackup returns the date (YYYY-MM-DD) of the most recent backup in
// dir, or ok=false when there are none. Backup filenames are date-stamped,
// so lexical order is chronological.
func LatestBackup(dir string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(dir, "data-*.json"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	sort.Strings(matches)
	base := filepath.Base(matches[len(matches)-1])
	return strings.TrimSuffix(strings.TrimPrefix(base, "data-"), ".json"), true
}

// DailyBackup copies the data file into dir as data-YYYY-MM-DD.json — at
// most once per day (a launch on a day that already has its backup is a
// no-op) — then prunes the oldest backups beyond keep. Call it once at
// startup, after a successful load; failures should be reported but never
// block the app from starting.
func DailyBackup(dataPath, dir string, keep int) error {
	data, err := os.ReadFile(dataPath)
	if os.IsNotExist(err) {
		return nil // nothing to back up yet
	}
	if err != nil {
		return err
	}

	dest := filepath.Join(dir, fmt.Sprintf("data-%s.json", time.Now().Format("2006-01-02")))
	if _, err := os.Stat(dest); err == nil {
		return nil // today's backup already exists
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return err
	}
	return pruneBackups(dir, keep)
}

// pruneBackups removes the oldest data-*.json files beyond keep. The
// date-stamped names sort chronologically, so lexicographic order is
// enough.
func pruneBackups(dir string, keep int) error {
	if keep < 1 {
		keep = 1
	}
	matches, err := filepath.Glob(filepath.Join(dir, "data-*.json"))
	if err != nil {
		return err
	}
	if len(matches) <= keep {
		return nil
	}
	sort.Strings(matches)
	for _, old := range matches[:len(matches)-keep] {
		if err := os.Remove(old); err != nil {
			return err
		}
	}
	return nil
}
