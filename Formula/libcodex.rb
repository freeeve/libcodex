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
  url "https://github.com/freeeve/libcodex/archive/refs/tags/v0.26.1.tar.gz"
  sha256 "119dfc50cc5fff58c42116d8d4dd1bd522413fc20176a853914a0f549f43d6e1"
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
