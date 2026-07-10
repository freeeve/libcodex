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
  url "https://github.com/freeeve/libcodex/archive/refs/tags/v0.28.0.tar.gz"
  sha256 "1e53298fc0be3dfb50532b9bc4d1c753e1e16a4e39bc603dba2d62988c5336a7"
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
