# Agent Instructions — questlog

## Data safety (critical — user data has been lost before)

The store is `~/.config/quests/data.json`. A past intermediate build silently wiped every quest's `jiraCodes`/`prs` fields; it went unnoticed because tests only ran against fresh isolated seeds.

- **Never run a dev/installed build against the real `~/.config/quests`** — always use an isolated `XDG_CONFIG_HOME` temp dir. `make install` means the user runs the result against real data.
- For ANY change to the persisted schema, a migration, or the Load/Save path: keep/extend the round-trip preservation test (`internal/store/store_test.go` `TestSavePreservesUserData`) — build a store with every user field populated, `Save`→`Load`→`Save`→`Load`, assert nothing dropped.
- Test migrations against data that ALREADY has the new-format fields populated, not just legacy fields being migrated.
- Daily backups: `~/.config/quests/backups/data-YYYY-MM-DD.json` (one per launch/day). Recovery = merge by quest `id`, preserving newer edits.

## UI consistency — apply changes to ALL similar surfaces (critical)

A UI change the user asks for must land on **every** surface that shows the same
kind of thing, not just the screen they pointed at. If duplication makes that
hard, decouple the shared component and route every surface through it. Full
rule + the list of parallel surfaces + shared components: **[docs/ui-consistency.md](docs/ui-consistency.md)**.

## UX — keyboard-first AND mouse

Every TUI interaction must work **both** keyboard-first and via mouse — never keyboard-only or click-only for any control.

- For multi-pane layouts, add explicit hotkeys to switch the active column/pane and to jump to specific sections.
- When adding any new pane/section/affordance, wire up both a keybinding (navigation, activation, section-jump, column-switch) and a mouse hit-test (click to focus/activate).
- Verify both paths headlessly (tmux send-keys + mouse events).

## Releases — always use the cut-release skill (never do it by hand)

ANY request to release, ship, publish, cut/tag a version, push a release, or
bump the version **must** go through the **`cut-release` skill** — load and
follow it before touching git; do not improvise the steps. It first confirms
the semver bump (major / minor / patch) with the user, then commits, tags with
a real changelog, cuts the GitHub Release, re-hashes the tarball, and mirrors
the Homebrew formula into the tap. Doing it manually has already shipped an
incomplete release (no GitHub Release, bare tag message) — so don't: open the
skill every time.
