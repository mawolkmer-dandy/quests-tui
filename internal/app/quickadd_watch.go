package app

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/mawolkmer-dandy/quests-tui/internal/quickadd"
)

// quickAddMsg is delivered when a new capture lands in the spool directory
// while the app is running, so it can be ingested live (see quickadd.Drain).
type quickAddMsg struct{}

// watchQuickAdd sets up a filesystem watcher on the quick-add spool and returns
// the command that waits for the first event. Returns nil (feature simply
// stays launch-only) if the watcher can't be created — capture never breaks,
// it just won't update a running app until relaunch.
func (m *Model) watchQuickAdd() tea.Cmd {
	dir := quickadd.Dir(filepath.Dir(m.path))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil
	}
	m.watcher = w
	return waitForQuickAdd(w)
}

// waitForQuickAdd blocks until a spool file is created (or the watcher closes),
// then reports a quickAddMsg. It's re-issued after each ingest to keep
// listening. The atomic temp+rename in quickadd.Enqueue means the finished
// entry always arrives as a Create on the ".json" name, never a partial write.
func waitForQuickAdd(w *fsnotify.Watcher) tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return nil
				}
				if ev.Op&(fsnotify.Create|fsnotify.Write) != 0 && filepath.Ext(ev.Name) == ".json" {
					return quickAddMsg{}
				}
			case _, ok := <-w.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
}
