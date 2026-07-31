# Agent Instructions — questlog

## Data safety (critical — user data has been lost before)

The store is `~/.config/quests/data.json`. A past intermediate build silently wiped every quest's `jiraCodes`/`prs` fields; it went unnoticed because tests only ran against fresh isolated seeds.

- **Never run a dev/installed build against the real `~/.config/quests`** — always use an isolated `XDG_CONFIG_HOME` temp dir. `make install` means the user runs the result against real data.
- For ANY change to the persisted schema, a migration, or the Load/Save path: keep/extend the round-trip preservation test (`internal/store/store_test.go` `TestSavePreservesUserData`) — build a store with every user field populated, `Save`→`Load`→`Save`→`Load`, assert nothing dropped.
- Test migrations against data that ALREADY has the new-format fields populated, not just legacy fields being migrated.
- Daily backups: `~/.config/quests/backups/data-YYYY-MM-DD.json` (one per launch/day). Recovery = merge by quest `id`, preserving newer edits.

## UX — keyboard-first AND mouse

Every TUI interaction must work **both** keyboard-first and via mouse — never keyboard-only or click-only for any control.

- For multi-pane layouts, add explicit hotkeys to switch the active column/pane and to jump to specific sections.
- When adding any new pane/section/affordance, wire up both a keybinding (navigation, activation, section-jump, column-switch) and a mouse hit-test (click to focus/activate).
- Verify both paths headlessly (tmux send-keys + mouse events).
