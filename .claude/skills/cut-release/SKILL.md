---
name: cut-release
description: Cut and publish a new version of Quests — bump, tag, push, re-hash the release tarball, and update the Homebrew formula in both the code repo and the tap. Use when the user asks to release, ship, publish, or bump the version.
---

# Cut a Quests release

Quests ships as a **source-build Homebrew tap** (pure Go, no cgo, no CI, no
signing). A release is a git tag plus a one-line `sha256` update mirrored into
the tap repo. Follow these steps in order — several have footguns that have
bitten before.

## Layout

| What | Where |
|------|-------|
| Code repo | `~/Repos/questlog` → `github.com/mawolkmer-dandy/quests-tui` |
| Tap repo | `~/Repos/homebrew-tap` → `github.com/mawolkmer-dandy/homebrew-tap` |
| Formula (source of truth) | `~/Repos/questlog/HomebrewFormula/quests.rb` |
| Formula (what brew reads) | `~/Repos/homebrew-tap/Formula/quests.rb` |
| Version stamping | `-ldflags "-X main.version=…"` in the Makefile + formula |

Install line users get: `brew install mawolkmer-dandy/tap/quests`

## Preconditions

1. Confirm the working tree is clean and validated:
   ```sh
   cd ~/Repos/questlog && gofmt -l . && go vet ./... && go build ./...
   ```
2. Confirm `gh auth status` is the **mawolkmer-dandy** account (owns both repos).
3. Pick the new version `vX.Y.Z` (semver). Ask the user if unsure.

## Steps

1. **Commit** any release-worthy changes and push `master`:
   ```sh
   cd ~/Repos/questlog && git add -A && git commit -m "…" && git push origin master
   ```

2. **Tag — must be annotated.** This repo's git config rejects lightweight
   tags (`git tag vX.Y.Z` fails with "no tag message?"). Always:
   ```sh
   git tag -a vX.Y.Z -m "Quests vX.Y.Z" && git push origin vX.Y.Z
   ```

3. **Hash the tarball — only after the tag is pushed.** GitHub generates the
   tarball lazily; a `curl` before it exists hashes the 404 page (the giveaway
   is `d5558cd419c8d46bdc958064cb97f963d1ea793866414c025906ec15033512ed`, the
   hash of GitHub's "Not Found" body). Download to a file and check the size:
   ```sh
   URL="https://github.com/mawolkmer-dandy/quests-tui/archive/refs/tags/vX.Y.Z.tar.gz"
   curl -sL "$URL" -o /tmp/quests.tar.gz
   test "$(wc -c < /tmp/quests.tar.gz)" -gt 1000 || echo "TARBALL NOT READY — retry"
   shasum -a 256 /tmp/quests.tar.gz
   ```

4. **Update the source-of-truth formula** `HomebrewFormula/quests.rb`:
   - `url` → the `vX.Y.Z` tarball URL
   - `sha256` → the hash from step 3
   Commit + push it to the code repo.

5. **Mirror the formula into the tap** and push:
   ```sh
   cp ~/Repos/questlog/HomebrewFormula/quests.rb ~/Repos/homebrew-tap/Formula/quests.rb
   cd ~/Repos/homebrew-tap && git add Formula/quests.rb \
     && git commit -m "quests vX.Y.Z" && git push origin master
   ```
   If the branch has no upstream, use `git push -u origin master` (a bare
   `git push` can silently no-op on a fresh clone).

6. **Verify end-to-end** exactly as another person would:
   ```sh
   brew uninstall quests 2>/dev/null; brew untap mawolkmer-dandy/tap 2>/dev/null
   brew install mawolkmer-dandy/tap/quests
   quests --version   # should print the new version
   ```

## Notes

- `main.version` defaults to `"dev"`; the Makefile stamps `git describe` and
  the formula stamps its `#{version}`. A clean tag build prints exactly `vX.Y.Z`.
- No GoReleaser/CI by design. If prebuilt binaries are ever wanted, see the
  "Later: prebuilt binaries" section of `RELEASING.md`.
