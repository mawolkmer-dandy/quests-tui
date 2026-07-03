---
name: tui-test
description: Drive the Quests TUI headlessly to verify behavior — launch it under tmux against an isolated fake config dir, send keystrokes, and capture the rendered screen. Use when verifying any UI/interaction change (navigation, editing, glyphs, scrolling, mouse) instead of guessing from the code.
---

# Headless testing for the Quests TUI

Quests is a full-screen Bubble Tea app — you can't see it in a normal tool
loop. Drive it under **tmux** and read the screen with `capture-pane`. This
harness is how every interaction change in this repo has been verified.

## Golden rule: never touch real data

Never run against the user's real `~/.config/quests`. Always point the app at
an isolated dir via `HOME` (the app resolves data under `$XDG_CONFIG_HOME` or
`$HOME/.config/quests`). Use the session scratchpad as a fake home:

```sh
FAKE_HOME="$(mktemp -d)"          # or a scratchpad path
BIN="$(cd ~/Repos/questlog && make build >/dev/null 2>&1 && pwd)/quests"
```

Build a fresh binary first (`make build`) so you're testing current code, not
the installed one.

## Launch, drive, capture

```sh
SESS="qtest_$$"
tmux new-session -d -s "$SESS" -x 120 -y 40 "HOME=$FAKE_HOME $BIN"
sleep 0.8                          # let it draw (and finish the intro animation)

# send keys — capitals of special keys are tmux key names:
tmux send-keys -t "$SESS" Down Down Enter
tmux send-keys -t "$SESS" "some typed text"
tmux send-keys -t "$SESS" Tab           # focus/open
tmux send-keys -t "$SESS" Escape        # close/back
sleep 0.3

# read the screen (-e keeps ANSI color so you can check styling; -p to stdout):
tmux capture-pane -e -p -t "$SESS"

tmux kill-session -t "$SESS"
```

## Gotchas learned the hard way

- **Re-capture before acting.** The intro animation and greeting eat early
  keystrokes; a label row like "Campaigns" can be selectable. Capture, read the
  real cursor position, *then* decide the next key — don't count keystrokes
  blind.
- **`cat -A` doesn't exist on macOS.** To inspect raw output use
  `capture-pane -e -p | cat` or pipe through `sed -n`/`awk`; for column checks,
  `awk '{print index($0,$2)}'`.
- **Glyph width / alignment:** to check that icons line up, print candidates in
  a fixed-width context and compare the column where the following word starts.
  `lipgloss.Width` (go-runewidth) reports the *logical* cell width; genuine
  visual alignment still depends on the user's terminal font.
- **Mouse-leak defense:** fast wheel scrolling can leak `[<…M` SGR fragments as
  text (bubbletea #1627). This repo filters them structurally + suppresses
  mouse-alphabet keys for 300ms after a wheel event. When testing scroll, send
  `WheelUp`/`WheelDown` via tmux and confirm no `[` fragments land in the body.
- **Disable the intro** for deterministic runs by seeding a config with
  `intro = false` in `$FAKE_HOME/.config/quests/config.toml`, or just
  `sleep` long enough for the animation to finish before sending keys.
- **Debug log:** set `QUESTS_DEBUG=1` in the launch env to write
  `quests-debug.log`; check it for stray/garbage messages.

## Seeding state

To test against known data, write `$FAKE_HOME/.config/quests/data.json`
directly (plain JSON, schema in `internal/model`), or drive the app to create
it. For import paths, use `--import-things --things-db <copy> --dry-run` against
a *copied* Things DB, never the live one.
