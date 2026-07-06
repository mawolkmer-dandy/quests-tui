package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/app"
	"github.com/mawolkmer-dandy/quests-tui/internal/config"
	"github.com/mawolkmer-dandy/quests-tui/internal/model"
	"github.com/mawolkmer-dandy/quests-tui/internal/quickadd"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/thingsimport"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

// version is stamped at build time via -ldflags "-X main.version=…" (see the
// Makefile and the Homebrew formula). It stays "dev" for a plain `go build`.
var version = "dev"

func main() {
	// Subcommands are dispatched before flag.Parse so their own flags (e.g.
	// `quests add --to Homestead …`) aren't swallowed as positionals.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "add":
			runAdd(os.Args[2:])
			return
		case "campaigns":
			runCampaigns(os.Args[2:])
			return
		}
	}

	showVersion := flag.Bool("version", false, "print the version and exit")
	initConfig := flag.Bool("init-config", false, "write a fresh ~/.config/quests/config.toml with every setting at its default (commented), then exit")
	force := flag.Bool("force", false, "with --init-config, overwrite an existing config file")
	importThings := flag.Bool("import-things", false, "import a local Things 3 database into Quests, then exit")
	thingsDB := flag.String("things-db", "", "path to a Things main.sqlite (default: auto-locate); use with --import-things")
	dryRun := flag.Bool("dry-run", false, "with --import-things, preview the import without writing anything")
	replace := flag.Bool("replace", false, "with --import-things, replace existing quests instead of appending (previous data is backed up first)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "quests — a keyboard-and-mouse quest journal TUI\n\nUsage:\n  quests                 launch the app\n  quests add <title…>    capture a quest from anywhere (see `quests add -h`)\n  quests campaigns       list campaign names (one per line)\n  quests --version       print the version\n  quests --init-config   write the default config file\n  quests --import-things import a local Things 3 database\n\nFlags:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("quests", version)
		return
	}

	dir, err := store.DefaultDir()
	if err != nil {
		fatal(err)
	}

	if *initConfig {
		path := config.Path(dir)
		if _, err := os.Stat(path); err == nil && !*force {
			fmt.Fprintf(os.Stderr, "quests: %s already exists — pass --force to overwrite\n", path)
			os.Exit(1)
		}
		if err := config.WriteSample(path); err != nil {
			fatal(err)
		}
		fmt.Println("quests: wrote default config to", path)
		return
	}

	if *importThings {
		res, err := thingsimport.Run(thingsimport.Options{
			DBPath:   *thingsDB,
			DataPath: filepath.Join(dir, "data.json"),
			DryRun:   *dryRun,
			Replace:  *replace,
		})
		if err != nil {
			fatal(err)
		}
		verb := "imported"
		if *dryRun {
			verb = "would import"
		}
		fmt.Printf("quests: %s %d campaigns and %d quests from %s\n", verb, res.Campaigns, res.Quests(), res.DBPath)
		fmt.Printf("        %d under campaigns · %d on the Questboard · %d in the Vault\n", res.CampaignTasks, res.Questboard, res.Vault)
		if *dryRun {
			fmt.Println("        (dry run — nothing written; drop --dry-run to apply)")
		} else if res.BackupPath != "" {
			fmt.Println("        backed up your previous data to", res.BackupPath)
		}
		return
	}

	// Resolve the terminal's background once, here, before Bubble Tea takes
	// over stdin below. This is a live terminal query (write an OSC-11
	// escape sequence, read the reply); done at any other point it races
	// Bubble Tea's own input reader for that reply, which is read as garbage
	// keystrokes and stalls the whole UI. lipgloss caches the result, so
	// every later AdaptiveColor use just reads this cached value.
	darkBg := lipgloss.HasDarkBackground()

	dataPath := filepath.Join(dir, "data.json")

	cfg, err := config.Load(config.Path(dir))
	if err != nil {
		// A broken config falls back to defaults; say so rather than dying.
		fmt.Fprintln(os.Stderr, "quests: ignoring invalid config:", err)
	}
	ui.ApplyTheme(ui.Theme{
		MainLight: cfg.Colors.MainLight, MainDark: cfg.Colors.MainDark,
		SideLight: cfg.Colors.SideLight, SideDark: cfg.Colors.SideDark,
		HeadingLight: cfg.Colors.HeadingLight, HeadingDark: cfg.Colors.HeadingDark,
		ImportantLight: cfg.Colors.ImportantLight, ImportantDark: cfg.Colors.ImportantDark,
		PriorityMediumLight: cfg.Colors.PriorityMediumLight, PriorityMediumDark: cfg.Colors.PriorityMediumDark,
	})
	ui.ApplyIcons(ui.IconSet{
		QuestOpen: cfg.Icons.QuestOpen, QuestActive: cfg.Icons.QuestActive, QuestDone: cfg.Icons.QuestDone,
		NoticeMain: cfg.Icons.NoticeMain, NoticeSide: cfg.Icons.NoticeSide,
		Important: cfg.Icons.Important, PriorityLow: cfg.Icons.PriorityLow,
		Expanded: cfg.Icons.Expanded, Collapsed: cfg.Icons.Collapsed,
	})
	ui.DoneToBottom = cfg.Behavior.DoneToBottom
	ui.MoveMainToTop = cfg.Behavior.MainToTop
	ui.MovePriorityToTop = cfg.Behavior.PriorityToTop
	ui.LowPriorityToBottom = cfg.Behavior.LowPriorityToBottom
	app.ApplyKeys(cfg.Keys)

	s, err := store.Load(dataPath)
	if err != nil {
		fatal(err)
	}

	// Ingest anything captured via `quests add` while the app was closed.
	if n := quickadd.Drain(dir, s); n > 0 {
		if err := store.Save(dataPath, s); err != nil {
			fmt.Fprintln(os.Stderr, "quests: failed to save quick-added tasks:", err)
		}
	}

	if cfg.Behavior.Backups {
		if err := store.DailyBackup(dataPath, filepath.Join(dir, "backups"), cfg.Behavior.BackupKeep); err != nil {
			fmt.Fprintln(os.Stderr, "quests: backup failed:", err)
		}
	}

	m := app.New(s, dataPath, darkBg, app.Options{
		QuestboardCollapsed: cfg.Behavior.QuestboardCollapsed,
		VaultCollapsed:      cfg.Behavior.VaultCollapsed,
		ShowHints:           cfg.Behavior.ShowHints,
		Intro:               cfg.Behavior.Intro,
		Greeting:            cfg.Behavior.Greeting,
	})

	if os.Getenv("QUESTS_DEBUG") != "" {
		logFile, err := tea.LogToFile("quests-debug.log", "debug")
		if err != nil {
			fatal(fmt.Errorf("failed to open debug log: %w", err))
		}
		defer logFile.Close()
		m.SetDebug(true)
	}

	// All-motion (not just cell-motion) mouse reporting is needed for hover
	// tips ("→ open (tab)", the Vault's "read only") — those need to know
	// where the mouse is even when no button is pressed.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "quests:", err)
	os.Exit(1)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "quests: "+format+"\n", args...)
	os.Exit(1)
}

// runAdd handles `quests add [flags] <title…>` — the quick-entry backend a
// global hotkey (Shortcuts, Raycast) calls. It never writes data.json; it
// spools the task (see package quickadd) for the running app or the next
// launch to ingest.
func runAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	to := fs.String("to", "", "campaign to file under (name; case-insensitive, prefix/substring match)")
	inbox := fs.Bool("inbox", false, "file on the Questboard (the default when --to is omitted)")
	asMain := fs.Bool("main", false, "mark as a main quest")
	asSide := fs.Bool("side", false, "mark as a side quest (the default)")
	important := fs.Bool("important", false, "flag as priority work")
	note := fs.String("note", "", "optional description; becomes the quest's body (newlines split into lines)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "quests add — capture a quest from anywhere\n\nUsage:\n  quests add [flags] <title…>\n  echo \"title\" | quests add [flags]\n\nFlags:")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	if *asMain && *asSide {
		fatalf("add: --main and --side are mutually exclusive")
	}
	if *to != "" && *inbox {
		fatalf("add: --to and --inbox are mutually exclusive")
	}

	title := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if title == "" {
		title = readStdinTitle()
	}
	if title == "" {
		fatalf("add: needs a task title (as arguments or piped on stdin)")
	}

	dir, err := store.DefaultDir()
	if err != nil {
		fatal(err)
	}
	s, err := store.Load(filepath.Join(dir, "data.json"))
	if err != nil {
		fatal(err)
	}

	projectID, target := "", "the Questboard"
	if *to != "" {
		p, err := resolveCampaign(s, *to)
		if err != nil {
			fatal(err)
		}
		projectID, target = p.ID, p.Name
	}

	questType := string(model.QuestTypeSide)
	if *asMain {
		questType = string(model.QuestTypeMain)
	}

	if err := quickadd.Enqueue(dir, quickadd.Entry{
		ID:        store.NewID(),
		Title:     title,
		Note:      *note,
		ProjectID: projectID,
		Type:      questType,
		Important: *important,
		CreatedAt: time.Now(),
	}); err != nil {
		fatal(err)
	}
	fmt.Printf("quests: captured %q → %s\n", title, target)
}

// runCampaigns prints one active campaign name per line — the list a picker
// (Shortcuts' "Choose from List", Raycast) feeds back into `add --to`.
func runCampaigns(_ []string) {
	dir, err := store.DefaultDir()
	if err != nil {
		fatal(err)
	}
	s, err := store.Load(filepath.Join(dir, "data.json"))
	if err != nil {
		fatal(err)
	}
	for _, name := range activeCampaignNames(s) {
		fmt.Println(name)
	}
}

func readStdinTitle() string {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return "" // a terminal, not a pipe — don't block waiting for input
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// resolveCampaign maps a user-typed name to a single non-archived campaign:
// an exact (case-insensitive) match wins; otherwise a unique substring match.
// Ambiguity or no match is a clear error listing what's available.
func resolveCampaign(s *store.Store, query string) (*model.Project, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	var exact, subs []*model.Project
	for i := range s.Projects {
		p := &s.Projects[i]
		if p.Archived {
			continue
		}
		name := strings.ToLower(p.Name)
		switch {
		case name == q:
			exact = append(exact, p)
		case strings.Contains(name, q):
			subs = append(subs, p)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	matches := exact
	if len(matches) == 0 {
		matches = subs
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		avail := activeCampaignNames(s)
		if len(avail) == 0 {
			return nil, fmt.Errorf("no campaign matching %q (there are no campaigns yet)", query)
		}
		return nil, fmt.Errorf("no campaign matching %q; available: %s", query, strings.Join(avail, ", "))
	}
	names := make([]string, len(matches))
	for i, p := range matches {
		names[i] = p.Name
	}
	return nil, fmt.Errorf("%q is ambiguous — matches: %s", query, strings.Join(names, ", "))
}

func activeCampaignNames(s *store.Store) []string {
	var names []string
	for _, p := range s.Projects {
		if !p.Archived {
			names = append(names, p.Name)
		}
	}
	sort.Strings(names)
	return names
}
