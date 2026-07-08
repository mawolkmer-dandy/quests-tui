package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"github.com/mawolkmer-dandy/quests-tui/internal/config"
)

// Almost every row is a live text field, so bare letters must default to
// text — every command below lives on a non-printable key or a Ctrl chord
// that bubbles/textinput's default keymap doesn't already claim (checked
// against ~/go/pkg/mod/.../bubbles@v1.0.0/textinput/textinput.go).
// App-level keys are intercepted before ever reaching the row's textinput,
// so this holds regardless of what textinput binds internally.
//
// Ctrl chords, not Alt/Option — on macOS, Option+letter is intercepted by
// the OS/terminal input layer and composes an accented character (Option+D
// → "∂") before it ever reaches the app as a distinguishable key event.
// Ctrl always sends a plain control byte, so it's reliable everywhere.
//
// Help labels are spelled out in plain ASCII ("Ctrl+D") rather than symbol
// glyphs (⌥, ⇥, ↵) — those fall back to unreadable tofu on fonts that don't
// ship them.
type KeyMap struct {
	Up              key.Binding
	Down            key.Binding
	Left            key.Binding
	Right           key.Binding
	MoveUp          key.Binding
	MoveDown        key.Binding
	Tab             key.Binding
	Enter           key.Binding
	Backspace       key.Binding
	ToggleActive    key.Binding
	ToggleDone      key.Binding
	ToggleImportant key.Binding
	ToggleVault     key.Binding
	ToggleType      key.Binding
	MoveProject     key.Binding
	Delete          key.Binding
	Undo            key.Binding
	SetOut          key.Binding
	Search          key.Binding
	Help            key.Binding
	ToggleHints     key.Binding
	Quit            key.Binding
}

var Keys = KeyMap{
	Up:              key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
	Down:            key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
	Left:            key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "cursor left")),
	Right:           key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "cursor right")),
	MoveUp:          key.NewBinding(key.WithKeys("shift+up"), key.WithHelp("Shift+↑", "move quest / campaign up")),
	MoveDown:        key.NewBinding(key.WithKeys("shift+down"), key.WithHelp("Shift+↓", "move quest / campaign down")),
	Tab:             key.NewBinding(key.WithKeys("tab"), key.WithHelp("Tab", "reveal / open")),
	Enter:           key.NewBinding(key.WithKeys("enter"), key.WithHelp("Enter", "new line")),
	Backspace:       key.NewBinding(key.WithKeys("backspace"), key.WithHelp("Backspace", "delete char / empty row")),
	ToggleActive:    key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("Ctrl+A", "toggle active / taken")),
	ToggleDone:      key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("Ctrl+D", "toggle done")),
	ToggleImportant: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("Ctrl+P", "cycle priority (med / high / low / none)")),
	ToggleVault:     key.NewBinding(key.WithKeys("ctrl+v"), key.WithHelp("Ctrl+V", "toggle vault")),
	ToggleType:      key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("Ctrl+T", "main / side")),
	MoveProject:     key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("Ctrl+O", "move to campaign")),
	Delete:          key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("Ctrl+X", "delete")),
	Undo:            key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("Ctrl+Z", "undo last change")),
	SetOut:          key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("Ctrl+G", "set out / return to tavern")),
	Search:          key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("Ctrl+F", "search")),
	Help:            key.NewBinding(key.WithKeys("f1"), key.WithHelp("F1", "help")),
	ToggleHints:     key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("Ctrl+K", "hide / show hover tips")),
	Quit:            key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("Ctrl+C", "quit")),
}

// ApplyKeys rebinds the rebindable shortcuts from config strings ("ctrl+d",
// "f1") — empty strings keep the default binding. Structural keys (arrows,
// Tab, Enter, Backspace, Esc, Ctrl+C) are not rebindable.
func ApplyKeys(k config.Keys) {
	rebind(&Keys.ToggleActive, k.ToggleActive)
	rebind(&Keys.ToggleDone, k.ToggleDone)
	rebind(&Keys.ToggleImportant, k.ToggleImportant)
	rebind(&Keys.ToggleVault, k.ToggleVault)
	rebind(&Keys.ToggleType, k.ToggleType)
	rebind(&Keys.MoveProject, k.MoveCampaign)
	rebind(&Keys.Delete, k.Delete)
	rebind(&Keys.Search, k.Search)
	rebind(&Keys.Help, k.Help)
	rebind(&Keys.ToggleHints, k.ToggleHints)
}

func rebind(b *key.Binding, keys string) {
	if keys == "" {
		return
	}
	desc := b.Help().Desc
	*b = key.NewBinding(key.WithKeys(keys), key.WithHelp(prettyKeyLabel(keys), desc))
}

// prettyKeyLabel turns "ctrl+d" into "Ctrl+D" and "f1" into "F1" for help
// text.
func prettyKeyLabel(keys string) string {
	parts := strings.Split(keys, "+")
	for i, p := range parts {
		switch {
		case p == "ctrl":
			parts[i] = "Ctrl"
		case p == "shift":
			parts[i] = "Shift"
		case p == "alt":
			parts[i] = "Alt"
		default:
			parts[i] = strings.ToUpper(p)
		}
	}
	return strings.Join(parts, "+")
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.MoveUp, k.MoveDown, k.Tab, k.Enter, k.Backspace},
		{k.ToggleActive, k.ToggleDone, k.ToggleImportant, k.ToggleVault, k.ToggleType, k.MoveProject},
		{k.Delete, k.Undo, k.SetOut, k.Search, k.Help, k.ToggleHints, k.Quit},
	}
}
