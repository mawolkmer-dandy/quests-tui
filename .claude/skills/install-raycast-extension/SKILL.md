---
name: install-raycast-extension
description: Install the Quests Raycast extension (Quick Add Quest + Add Quest) into the user's Raycast app. Use when the user asks to install, set up, or re-import the Raycast quick-entry commands.
---

# Install the Quests Raycast extension

The extension lives in `integrations/raycast/` and adds two commands:
**Quick Add Quest** (no-view, inline title → Questboard) and **Add Quest**
(full form with campaign/type/description/priority). Both shell out to the
`quests` CLI's `add`/`campaigns` subcommands.

## Preconditions

1. **The `quests` binary must have the `add`/`campaigns` subcommands.** Verify:
   ```sh
   quests campaigns >/dev/null 2>&1 && echo ok || echo "missing subcommands"
   ```
   If missing, the user's `quests` is an older build. Fix by installing the
   current one: `cd <repo> && make install` (dev) or `brew upgrade quests`
   once the feature is released. The extension resolves `~/go/bin/quests`
   first, then `/opt/homebrew/bin`, then PATH — so `make install` is enough
   for local use.
2. **Node + npm** present (`node --version`).
3. **Raycast must be running** for `ray develop` to register the commands.

## Steps

```sh
cd <repo>/integrations/raycast
npm install
npx ray build -e dist      # verify it compiles before importing
```

If `ray build` fails with `Type 'bigint' is not assignable to ReactNode` (or
similar JSX type errors), the `@types/react` version is out of sync with what
`@raycast/api` bundles. Align them:
```sh
# match @types/react to @raycast/api's expected version:
grep '"@types/react"' node_modules/@raycast/api/package.json
npm install --save-dev @types/react@<that-version>
```

Then register the commands into Raycast:
```sh
npx ray develop            # long-running watcher — imports + hot-reloads
```

`ray develop` does not exit on its own (it's a file watcher + log streamer).
Run it in the background, confirm the commands appear in Raycast, then it can
be stopped — a development extension stays installed after the watcher exits.
Tell the user to assign hotkeys in **Raycast → Extensions → Quests**, and that
they only need to re-run `ray develop` to pick up future code changes.

## Notes

- Don't move or delete `integrations/raycast/node_modules` — dev extensions run
  from the built files in that folder.
- A Raycast update can occasionally drop development extensions; re-running
  `ray develop` re-imports them.
