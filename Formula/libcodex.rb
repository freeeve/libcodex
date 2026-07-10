# Homebrew formula for the libcodex CLI. This repository doubles as its own tap:
#
#   brew tap freeeve/libcodex https://github.com/freeeve/libcodex
#   brew install freeeve/libcodex/libcodex
#
# The formula builds from the tagged source with the module's own Go toolchain,
# so it carries no third-party dependencies. Bump `url`/`sha256` on each release
# (sha256 is that of the GitHub source tarball).
class Libcodex < Formula
  desc "Inspect and convert MARC / BIBFRAME bibliographic records"
  homepage "https://github.com/freeeve/libcodex"
  url "https://github.com/freeeve/libcodex/archive/refs/tags/v0.23.0.tar.gz"
  sha256 "a46d260ea3d856cba620352d5c9993d733a711f79fea4b65cdcb9459f4a7c113"
  license "MIT"
  head "https://github.com/freeeve/libcodex.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", "-trimpath", "-ldflags", ldflags, "-o", bin/"libcodex", "./cmd/libcodex"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/libcodex version")
  end
end
