# Quests — Raycast extension

A Raycast **Add Quest** command: a keyboard-first form to capture a quest into
the Questboard or a campaign, backed by the `quests add` CLI.

- **Title** (required) · **Description** (optional, → the quest body)
- **Campaign** dropdown, populated live from `quests campaigns`
- **Type** Side/Main · **Important** checkbox
- Tab between fields; **⌘↵** submits.

It shells out to the `quests` binary, preferring the `make install` build at
`~/go/bin/quests` (which has the `add`/`campaigns` subcommands), then falling
back to `/opt/homebrew/bin` and `/usr/local/bin`.

## Install (including for other people)

**Prerequisite — the `quests` CLI must be installed** (it provides the
`add`/`campaigns` subcommands this extension calls):

```sh
brew install mawolkmer-dandy/tap/quests
```

Then import the extension into Raycast:

```sh
git clone https://github.com/mawolkmer-dandy/quests-tui
cd quests-tui/integrations/raycast
npm install
npx ray develop     # registers "Quick Add Quest" + "Add Quest" in Raycast
```

You can quit `ray develop` (Ctrl-C) once the commands appear — they stay
installed as a development extension; re-run it only to pick up code changes.
Assign hotkeys in Raycast → Extensions → Quests.

`ray develop` registers **Add Quest** in Raycast and hot-reloads on save. Assign
a global hotkey in Raycast → Extensions → Quests → Add Quest → Record Hotkey.

To produce a static build without the dev server:

```sh
npm run build       # → dist/
```

## Notes

- Captures are spooled by the CLI (`~/.config/quests/quick-add/`) and ingested
  by the running app or the next launch — see the repo's Quick entry docs.
- Once the quick-add feature ships to the Homebrew build, the `~/go/bin`
  preference no longer matters; any `quests` on PATH with the subcommands works.
