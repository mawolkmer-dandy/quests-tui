package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mawolkmer-dandy/quests-tui/internal/app"
	"github.com/mawolkmer-dandy/quests-tui/internal/config"
	"github.com/mawolkmer-dandy/quests-tui/internal/store"
	"github.com/mawolkmer-dandy/quests-tui/internal/thingsimport"
	"github.com/mawolkmer-dandy/quests-tui/internal/ui"
)

func main() {
	initConfig := flag.Bool("init-config", false, "write a fresh ~/.config/quests/config.toml with every setting at its default (commented), then exit")
	force := flag.Bool("force", false, "with --init-config, overwrite an existing config file")
	importThings := flag.Bool("import-things", false, "import a local Things 3 database into Quests, then exit")
	thingsDB := flag.String("things-db", "", "path to a Things main.sqlite (default: auto-locate); use with --import-things")
	dryRun := flag.Bool("dry-run", false, "with --import-things, preview the import without writing anything")
	replace := flag.Bool("replace", false, "with --import-things, replace existing quests instead of appending (previous data is backed up first)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "quests — a keyboard-and-mouse quest journal TUI\n\nUsage:\n  quests                 launch the app\n  quests --init-config   write the default config file\n  quests --import-things import a local Things 3 database\n\nFlags:")
		flag.PrintDefaults()
	}
	flag.Parse()

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
	})
	ui.ApplyIcons(ui.IconSet{
		QuestOpen: cfg.Icons.QuestOpen, QuestActive: cfg.Icons.QuestActive, QuestDone: cfg.Icons.QuestDone,
		NoticeMain: cfg.Icons.NoticeMain, NoticeSide: cfg.Icons.NoticeSide,
		Important: cfg.Icons.Important,
		Expanded:  cfg.Icons.Expanded, Collapsed: cfg.Icons.Collapsed,
	})
	ui.DoneToBottom = cfg.Behavior.DoneToBottom
	ui.MoveMainToTop = cfg.Behavior.MainToTop
	ui.MovePriorityToTop = cfg.Behavior.PriorityToTop
	app.ApplyKeys(cfg.Keys)

	s, err := store.Load(dataPath)
	if err != nil {
		fatal(err)
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
