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

2. **Write the changelog, then tag with it.** Users read these notes to know
   what changed, so the tag message must be a real summary — never just
   "Quests vX.Y.Z". First review what shipped since the previous tag:
   ```sh
   PREV=$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null)   # previous release tag
   git log --oneline "$PREV"..HEAD
   ```
   Turn that into a short, user-facing changelog — 2–6 bullets in plain
   language (what changed for *users*, not commit-by-commit), grouped as
   Added / Changed / Fixed when it helps. Don't just paste commit subjects;
   summarize. Then create an **annotated** tag whose message is the changelog
   (this repo's git config rejects lightweight tags — bare `git tag vX.Y.Z`
   fails with "no tag message?"):
   ```sh
   git tag -a vX.Y.Z -F - <<'EOF'
   Quests vX.Y.Z

   - <what changed, in user terms>
   - <another change>
   EOF
   git push origin vX.Y.Z
   ```
   Then publish it as a GitHub Release so the notes show up on the repo's
   Releases page (reuse the tag message). **Use `contents:subject` +
   `contents:body`, NOT `%(contents)`** — this repo signs tags, and
   `%(contents)` appends the PGP signature block into the release notes (ugly,
   though harmless — a signature is public, not a secret):
   ```sh
   git tag -l --format='%(contents:subject)%0a%0a%(contents:body)' vX.Y.Z \
     | gh release create vX.Y.Z --repo mawolkmer-dandy/quests-tui \
       --title "vX.Y.Z" --notes-file -
   ```
   Keep a running `CHANGELOG.md` in the repo in sync if one exists.

   **Always review the published release notes and fix if needed.** Immediately
   after creating the release, read back what actually rendered and correct
   anything wrong — never assume the submit was clean:
   ```sh
   gh release view vX.Y.Z --repo mawolkmer-dandy/quests-tui --json body -q '.body'
   ```
   Check for: a stray PGP signature block, truncated/garbled bullets, wrong
   version, or empty notes. Fix in place with
   `gh release edit vX.Y.Z --repo mawolkmer-dandy/quests-tui --notes-file -`.

   > **Write the changelog *before* the first push of the tag.** Never
   > re-annotate and force-push an already-published tag — that's a
   > destructive rewrite and is blocked. If notes are missing or wrong on a
   > tag that's already pushed, fix the **Release**, not the tag:
   > `gh release edit vX.Y.Z --notes "…"` (or `gh release create` if no
   > release exists yet).

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
