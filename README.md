# Quests

A personal, keyboard-and-mouse TUI for tracking work as quests — a single
fluid, collapsible outline, in the spirit of Things' Logbook. Invoke it
with `quests`.

- Every quest shows one diamond icon that does two jobs at once: its
  **shape** is progress, its **color** is type. Gold = main quest, blue =
  side quest (promote with `Ctrl+T`) — borrowed from Skyrim's own compass
  marker convention rather than a generic todo-app star/checkbox.
  - `◇` open · `⬖` active (`Ctrl+A`) · `◆` done (`Ctrl+D`) — each is a
    toggle: pressing it again reverts to open, and setting one always
    clears whichever of the others was set. If you want a quest gone
    rather than marked anything, delete it (`Ctrl+X`) — there's no
    separate "canceled" status.
  - A quest still sitting on the Questboard shows `!` (main) or `?` (side)
    instead — the classic "quest available" notice mark. It's listing-only:
    no active/done state applies until it's picked up (`Ctrl+O` to move it
    to a campaign).
  - Objectives inside a quest's detail view reuse the same `◇`/`◆` shapes,
    just muted instead of colored (they don't have a main/side axis).
  - Flag priority work with `Ctrl+P` — a red `↑` appears to the left of the
    quest. It's an orthogonal axis from type/status (a main quest can be
    important, a Questboard notice can be important), toggled off by
    pressing again. The glyph and color are configurable, and like the
    other status toggles it's blocked (read-only) inside the Vault.
- Each quest belongs to a **campaign**. Being **active** just means the
  quest currently has that explicit status — separately, a quest is
  implicitly "live" simply by sitting under a non-archived campaign at all,
  regardless of its status.
- The **Questboard** (no campaign yet) sits at the top, always starting
  collapsed. The **Vault** sits below the campaigns — a single place for
  anything not currently active, whether that's a quest you've deliberately
  parked or a whole campaign you've archived (shown nested and indented
  inside it, not as its own peer section). Quests only enter the Vault via
  `Ctrl+V` — `Enter` never creates one there directly. The Vault is
  read-only: a vaulted quest keeps whatever done/active status it had when
  it was sent there, and `Ctrl+D`/`Ctrl+A` (or clicking its checkbox) are
  no-ops on it — trying either shows a brief inline "vault is read-only"
  warning in place of the title for a couple of seconds. Pull it back out
  with `Ctrl+V` first to change anything.
- Focus a quest or campaign (`Tab`) to edit its outline in a full-screen,
  vertically centered view — not a popup, just that one thing with
  everything else cleared away and a `← back (esc)` line at the top (`F1`
  on the right opens a help page specific to this view — formatting and
  every shortcut usable here). Type `# ` to start a heading, `- ` to start
  an objective (checkbox), anything else is plain text — classified live as
  you type. Line editing behaves like a normal multiline editor, modeled
  on Obsidian/Notion list editing:
  - `Enter` mid-line splits it — the text after the cursor moves to a new
    line, and splitting inside an objective carries the `- ` onto it
    (splitting at the end of one just continues the list). `Enter` on an
    empty `- `/`# ` line exits the list instead: the marker clears.
  - `Backspace` at the start of a marked line first strips its marker
    (un-bullets it); at the start of a plain line it merges into the line
    above, cursor landing at the junction. `Delete` at the end of a line
    pulls the next line up (dropping its marker).
  - `↑`/`↓` keep the cursor column; pasting multi-line text splits it into
    real lines, with `- `/`# ` prefixes classified as usual.
  - `Tab`/`Shift+Tab` indent/outdent a line to nest objectives (a line
    can't indent more than one level past the line above it). New lines
    inherit the current indent.
  - Lines longer than the view soft-wrap onto continuation rows (at word
    boundaries, indented to align under the text) — the cursor, selection
    highlighting, and mouse clicks all follow the wrapping. If the whole
    thing is taller than the screen it scrolls to keep the cursor in view.
  Campaigns get the same focused view as quests, description and all — it
  also lists the campaign's quests below the description with every normal
  quest interaction still available (add, delete, toggle done/type,
  reorder, move to another campaign), and `Tab` on one of those drills into
  that quest's own focused view (`Esc` steps back out to the campaign, not
  straight to the main outline).
- `Enter` toggles open/closed on anything collapsible: a campaign, the
  Questboard, the Vault, or the "Campaigns" label itself (which
  collapses/expands every campaign at once). `Tab` opens a full-screen
  focused view — of a quest, a campaign, **or the Questboard / Vault** (for
  more room to work through them; `Esc` returns to the outline). It's only a
  no-op on the "Campaigns" label. Whatever's under the cursor (or the mouse)
  shows its available actions inline, e.g. `↑ collapse (enter)  → open (tab)`
  on a campaign — see "Hover tips" below.
- A "+ New Quest" row sits at the end of every campaign's quest list and at
  the end of the Questboard, mirroring "+ New Campaign" — `Enter` or a
  click adds a quest right there. The Vault doesn't get one; quests only
  land in it via `Ctrl+V`.
- Completing a quest doesn't reorder the list — it stays where it was
  instead of jumping to the bottom (configurable; see `done_to_bottom`).

Data is stored locally in `~/.config/quests/data.json` (or under
`$XDG_CONFIG_HOME` if set) — plain JSON, no DB, no account or sync; the
exact path is also shown in the `F1` help overlay. A daily backup lands in
`~/.config/quests/backups/` (see Configuration), and data from an earlier
location (the pre-XDG `~/.quests`, or the original `~/.questlog`) is
migrated automatically on first launch, leaving the original untouched.

## Install

Via Homebrew (recommended on macOS):

```sh
brew install mawolkmer-dandy/tap/quests
```

`brew upgrade` keeps it current. See [RELEASING.md](RELEASING.md) for how the
tap is maintained.

With Go:

```sh
go install github.com/mawolkmer-dandy/quests-tui/cmd/quests@latest
```

From a clone:

```sh
make install       # installs `quests` into your GOBIN
# or
make build && ./quests
```

## Quick entry — capture from anywhere

`quests add` files a new quest without opening the UI, so a global hotkey
(macOS Shortcuts, Raycast, Alfred…) can capture a thought in a second:

```sh
quests add "Buy milk"                          # → the Questboard (inbox)
quests add --to Homestead "Fix the roof"       # → a campaign (name match:
                                               #   exact, else unique substring)
quests add --to home --main --important "…"    # main quest, flagged priority
echo "piped title" | quests add                # title can come from stdin
quests campaigns                               # list campaign names (one per
                                               #   line) to drive a picker
```

Captures never write `data.json` directly — they're spooled into
`~/.config/quests/quick-add/`, so they can't race a running app. The running
app ingests them live; if it's closed, the next launch picks them up. Nothing
is lost either way.

To wire a **macOS Shortcut** ("Quick Quest"):

1. *Ask for Input* (Text) → the quest title.
2. *(optional, to route to a campaign)* *Run Shell Script*
   `~/go/bin/quests campaigns` → *Split Text* by New Lines → *Choose from List*.
3. *Run Shell Script*:
   `~/go/bin/quests add --to "$(Chosen Item)" "$(Provided Input)"`
   (drop `--to …` to always file on the Questboard).
4. Assign a keyboard shortcut to the Shortcut. Use the binary's full path
   (`which quests`) since Shortcuts doesn't inherit your shell `PATH`.

The same `quests add` backs a Raycast script command just as well.

## Configuration

`~/.config/quests/config.toml` — a fully commented sample with every default
is written on first run; every setting is optional. Run `quests --init-config`
to (re)generate that annotated file on demand — it refuses to clobber an
existing config unless you add `--force`. Highlights:

| Section | Setting | Default | What it does |
|---|---|---|---|
| `[behavior]` | `done_to_bottom` | `true` | sink completed quests to the bottom of their campaign |
| | `main_to_top` | `true` | float main quests to the top |
| | `priority_to_top` | `true` | float important quests to the top (outranks `main_to_top`; `done_to_bottom` still wins) |
| | `questboard_collapsed` / `vault_collapsed` | `true` / `true` | start the Questboard / Vault collapsed |
| | `show_hints` | `true` | inline action hints on launch (`Ctrl+K` still toggles) |
| | `intro` | `true` | play the startup logo animation |
| | `greeting` | `""` | fix the subtitle; empty picks a random tavern greeting |
| | `backups` / `backup_keep` | `true` / `14` | daily copy of `data.json` into `backups/`, keeping the last N days |
| `[colors]` | `main_*`, `side_*`, `heading_*`, `important_*` | Catppuccin-ish | hex accent colors, with light/dark-terminal variants |
| `[icons]` | `quest_*`, `notice_*`, `important`, `expanded`, `collapsed` | `◇⬖◆!?↑▾▸` | every glyph is swappable |
| `[keys]` | `toggle_done`, `search`, … | `ctrl+d`, `ctrl+f`, … | rebind any shortcut in bubbletea key syntax; arrows/Tab/Enter/Backspace/Esc/Ctrl+C are structural and fixed |

## Importing from Things 3

`quests --import-things` reads a local [Things 3](https://culturedcode.com/things/)
database and merges it into your quests (appending — it never deletes what
you already have, and it snapshots `data.json` into `backups/` first). Add
`--dry-run` to preview the counts without writing anything, or `--replace`
to wipe your current quests and start clean from the import (handy for
re-importing — your old data is still saved to `backups/` first).

The mapping:

| Things | Quests |
|---|---|
| Area | Campaign |
| Project | a side quest under that area's campaign; its notes → body, its headings → `# ` lines (blank line above/below), its to-dos → `- ` objectives (checked if completed), nested one level under their heading |
| Loose to-do in an area | a side quest under that campaign |
| Inbox / Someday item | a Questboard quest |
| Logbook item (done/cancelled) | a Vault quest (kept done if completed) |

A to-do's checklist items and notes come across too, nested one level under
their parent to-do. Everything imports as a **side** quest (Things has no
main/side distinction).

Things keeps its database in a sandboxed folder macOS guards, so one of
these is needed:

- **Grant Full Disk Access** to your terminal (System Settings → Privacy &
  Security → Full Disk Access), then run `quests --import-things`; or
- **Copy the database** somewhere readable and point at it:
  `quests --import-things --things-db /path/to/main.sqlite` (find it under
  `~/Library/Group Containers/JLMPQHK86H.com.culturedcode.ThingsMac/…/Things Database.thingsdatabase/main.sqlite`).

## The outline

Everything is centered on screen, under a small banner (title plus a
randomized tavern-flavored greeting, re-rolled every launch), with just a
right-aligned "F1 help" pointer pinned to the bottom (the full keybinding
list and a glossary of terms live in the `F1` overlay, not the footer):

```
                              QUESTS
                        Welcome to the inn.

▾ Questboard (1)
  ! A stranger has a job for you

Campaigns

▾ Homestead                                     ◔ 1/3
  ◆ Repair the phial                         ✓ 2/3
  ⬖ Clear the cellar
  ◇ Tune the forge
  + New Quest

▸ Garden (collapsed)                            ○ 0/2

+ New Campaign

▾ Vault (2)
  ◇ Learn woodworking                        [Garden]
  ▸ Old Side Project                              ○ 3/3
                                                        F1 help
```

Archived campaigns render indented one level further than active ones (see
`Old Side Project` above) so it's visually unambiguous that they're nested
inside the Vault — collapsing the Vault hides them along with everything
else in it.

Every row is a live text field — arrow onto any campaign or quest and just
type; leave a title blank and it stays blank, no placeholder text. `▾`/`▸`
mark anything collapsible, and a blank line always separates one
campaign/section from the next. `Questboard` always starts collapsed.

## Keybindings

Almost everything is a text field, so commands live on non-printable keys
or `Ctrl` chords rather than bare letters — and `Ctrl`, not `Alt`/`Option`:
on macOS, Option+letter is intercepted by the OS/terminal and composes an
accented character (Option+D → "∂") before it ever reaches the app, so it's
unreliable for shortcuts. Ctrl always sends a plain control byte. Help text
also spells keys out in plain ASCII (`Ctrl+D`, not `⌥d`) since symbol
glyphs fall back to unreadable tofu on fonts that don't ship them.

| Key | Action |
|---|---|
| `↑ / ↓` | move cursor to the previous/next row (commits the current edit) |
| `PgUp / PgDn` | jump half a page; the mouse wheel scrolls a line at a time (both here and in a focused quest/campaign). A muted `···` at the top/bottom edge marks content beyond the fold. |
| `← / →` | move the text cursor within the current row's title |
| `Shift+← / Shift+→ / Shift+Home / Shift+End` | select text in the current field — see "Text selection" below |
| `Shift+↑ / Shift+↓` | **reorder** — move the quest/campaign under the cursor among its siblings |
| `Tab` | **focus** — open a full-screen view of a quest, campaign, or section (Questboard/Vault); no-op on the "Campaigns" label |
| `Enter` | insert a new row right after the cursor, ready to type — on a campaign, `Questboard`, `Vault`, or the "Campaigns" label it toggles collapse instead |
| `Backspace` on an empty title | delete that row, move cursor to the previous line |
| `Ctrl+A` | toggle quest active |
| `Ctrl+D` | toggle quest done |
| `Ctrl+P` | toggle important — flags priority work with a red `↑` |
| `Ctrl+V` | toggle vault — parks a quest, or archives the campaign under the cursor |
| `Ctrl+T` | toggle quest type, main ↔ side |
| `Ctrl+O` | move quest to a different campaign |
| `Ctrl+X` | delete the row under the cursor, with an inline y/n prompt (deleting a campaign deletes its quests too) |
| `Ctrl+F` | live search by title |
| `F1` | help overlay (Campaigns/Vault/Questboard glossary + keys) — inside a focused quest/campaign, opens that view's own help instead |
| `Ctrl+K` | hide/show hover tips (the action hints and the Vault's `(read only)` badge) — the delete y/n prompt isn't affected |
| `Esc` | cancel edit / back out of a focused quest or campaign / close a dialog / clear search |
| `Ctrl+C` | quit (bare `q` types the letter q — it's text now) |

`Shift+↑`/`Shift+↓` reorders within a **sort tier**: a quest swaps with the
nearest quest in the same tier (see the `*_to_top`/`done_to_bottom`
settings) in the same campaign, so it can't jump a side above a main or a
main above a priority when those groupings are on — with all of them off
there's one tier, so anything can move past anything. A campaign likewise
swaps with the nearest campaign in the same list (active vs. archived in
the Vault don't interleave). Hitting a boundary before finding a valid
sibling is a no-op rather than crossing it. When a toggle moves a quest
*between* tiers (e.g. un-maining it while `main_to_top` is on), it floats
to the top of the tier it lands in rather than keeping a now-arbitrary
spot.

`Ctrl+X` always confirms inline, right on the row itself, rather than a
popup — `delete this quest? y/n`, or `delete this campaign and its N
quest(s)? y/n` for a campaign. Backspacing an empty quest title normally
deletes it outright too, but if the quest has details (a non-empty body
from its expanded view), that's risky to do silently, so it arms the same
inline prompt instead of deleting right away.

Inside a focused quest or campaign: `Enter` adds a new outline line,
`Ctrl+D` toggles the objective under the cursor, `Esc` backs out (autosaves,
no explicit save step).

Mouse: click any row to move the cursor there, click a `▾`/`▸` caret to
toggle collapse, click a quest's checkbox to toggle done directly, click
"+ New Quest"/"+ New Campaign" to add a row, scroll to move the cursor.

## Text selection

Every text field — a row title, a body line, search — supports selecting a
range of text, e.g. to grab a URL out of a quest's description:

- **Keyboard**: `Shift+←`/`Shift+→` extend the selection one character at a
  time from wherever the cursor was; `Shift+Home`/`Shift+End` extend it to
  the start/end of the line. Inside a focused quest/campaign,
  `Shift+↑`/`Shift+↓` extend it across body lines.
- **Mouse**: click and drag across the text — inside a focused
  quest/campaign, dragging across lines selects all of them.

A selection that spans multiple lines is **copy-only**: typing or
`Backspace` just drops it (then behaves normally) rather than deleting
across lines — cross-line editing isn't worth its edge cases when the
point is grabbing text. The copied lines are joined with newlines.

A selection is highlighted with a background color and copied to the
system clipboard the moment it changes — there's no separate copy key,
selecting is copying. Typing or pressing `Backspace`/`Delete` replaces the
selected text, like any normal text editor; any other key (an arrow without
Shift, `Tab`, `Enter`, etc.) just drops the highlight without touching the
text.

Copying briefly swaps the "F1 help" pointer — the footer in the main
outline, or the header of a focused quest/campaign view — for a
"● copied to clipboard" indicator, for about a second and a half.

## Hover tips

Whatever's under the cursor — keyboard or mouse, either counts — shows its
available actions inline, right after it, as `<icon> <verb> (<key>)`:

| Hint | Meaning |
|---|---|
| `← back (esc)` | the header of a focused quest/campaign view |
| `↑ collapse (enter)` / `↓ expand (enter)` | on a campaign, a section, or the "Campaigns" label |
| `→ open (tab)` | on any campaign, quest, or section |

A campaign or section shows both collapse and open together, since `Enter`
and `Tab` both do something there. Hovering the Vault or anything inside it (mouse
only) additionally shows `(read only)` next to the Vault's own title.
`Ctrl+K` hides all of these — the delete y/n prompt and the vault-read-only
warning aren't hover tips and always show regardless.

Every hint is also a **button**: clicking `→ open (tab)` opens the row,
clicking `↑ collapse (enter)` collapses it, and so on — same for a focused
view's `← back (esc)` and `F1 help`. Hiding hints with `Ctrl+K` also
removes the click targets.

## Startup

Launching the app plays a brief (well under a second) intro, entirely in
place: the outline, footer, and everything else are already laid out
exactly as they'll stay, and only the logo animates — a yellow shine sweeps
across `QUESTS` (the letters themselves stay the same plain color the
whole time, so there's no jump when it's over) while the subtitle types in
underneath, character by character. Pressing any key skips straight to the
end. There's no other animation in the app — every interaction (toggling
done, deleting a row, etc.) takes effect immediately.

## Debugging

Run with `QUESTS_DEBUG=1 quests` to log every message Bubble Tea
receives (type + time since the previous message) to `quests-debug.log`
via `tea.LogToFile` — useful if something feels laggy or input seems to
misfire.
