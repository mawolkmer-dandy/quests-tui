# Releasing Quests

Quests is distributed as a Homebrew tap that **builds from source** (pure Go,
no cgo) — so there's no binary signing, notarization, or CI to maintain. A
release is just a git tag plus a one-line hash update in the tap.

## One-time setup

You need two public repos under the **same GitHub account** as the module path
(`github.com/mawolkmer-dandy/...`):

| Repo | Purpose |
|------|---------|
| `quests-tui` | this codebase |
| `homebrew-tap` | holds `Formula/quests.rb` — what `brew` reads |

```sh
# from ~/Repos/questlog, with the first commit already made:
gh repo create mawolkmer-dandy/quests-tui --public --source=. --remote=origin --push

# the tap (can be an empty repo to start):
gh repo create mawolkmer-dandy/homebrew-tap --public --clone
```

The tap repo name **must** be `homebrew-tap` — that's the `<user>/tap`
shorthand Homebrew expects (`brew install mawolkmer-dandy/tap/quests`).

## Cutting a release

1. Tag and push in `quests-tui`:

   ```sh
   git tag v1.0.0
   git push origin v1.0.0
   ```

2. Hash the release tarball GitHub generates for that tag:

   ```sh
   curl -sL https://github.com/mawolkmer-dandy/quests-tui/archive/refs/tags/v1.0.0.tar.gz | shasum -a 256
   ```

3. Update `url` (the version) and `sha256` in `HomebrewFormula/quests.rb`,
   then copy it into the tap repo as `Formula/quests.rb` and commit:

   ```sh
   cp HomebrewFormula/quests.rb ../homebrew-tap/Formula/quests.rb
   cd ../homebrew-tap && git add Formula/quests.rb \
     && git commit -m "quests v1.0.0" && git push
   ```

Users pick it up with `brew install mawolkmer-dandy/tap/quests` (first time) or
`brew upgrade quests` (thereafter).

## Verifying a release locally

```sh
brew install --build-from-source ../homebrew-tap/Formula/quests.rb
command -v quests   # confirm it landed on PATH, then `brew uninstall quests`
```

## Later: prebuilt binaries

Source builds pull Go as a build dependency and compile on the user's machine
(~10–20s). If that ever feels heavy, add [GoReleaser](https://goreleaser.com)
to cross-compile darwin/linux × arm64/amd64, attach them to the GitHub Release,
and have it auto-update the tap formula to a `bottle`/binary install. Not
needed for v1.
