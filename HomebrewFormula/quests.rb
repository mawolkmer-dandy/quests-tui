# Homebrew formula for Quests.
#
# This file is the source of truth. To publish it through your tap:
#
#   1. Create a public repo named `homebrew-tap` on your GitHub account.
#   2. Copy this file into it as `Formula/quests.rb`.
#   3. Users then install with:
#        brew install mawolkmer-dandy/tap/quests
#      (shorthand for `brew tap mawolkmer-dandy/tap && brew install quests`)
#
# Releasing a new version:
#   1. Bump the version, then in this repo:  git tag vX.Y.Z && git push --tags
#   2. Compute the tarball hash:
#        curl -sL https://github.com/mawolkmer-dandy/quests-tui/archive/refs/tags/vX.Y.Z.tar.gz | shasum -a 256
#   3. Update `url` + `sha256` below, copy into homebrew-tap, commit.
#
# `depends_on "go" => :build` means brew builds from source on the user's
# machine (Quests is pure Go, no cgo) — no signing/notarization needed, and
# `brew install`/`brew upgrade` clears the Gatekeeper quarantine for you.
class Quests < Formula
  desc "Quest journal TUI — track personal work as quests inside campaigns"
  homepage "https://github.com/mawolkmer-dandy/quests-tui"
  url "https://github.com/mawolkmer-dandy/quests-tui/archive/refs/tags/v1.1.1.tar.gz"
  sha256 "461a0871532227786f8d45c9d908d483489086543bc5ea60656d6b4951c8cf11"
  license "MIT"
  head "https://github.com/mawolkmer-dandy/quests-tui.git", branch: "master"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags, output: bin/"quests"), "./cmd/quests"
  end

  test do
    assert_predicate bin/"quests", :exist?
  end
end
