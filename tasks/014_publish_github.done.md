# 014 — Publish: public GitHub repo + CI + initial commit

## Goal
Initialize git, push to a new PUBLIC GitHub repo `freeeve/libcodex`, and set up
CI. Do this AFTER the other tasks are complete.

## Scope
- `git init`; add a Go `.gitignore`; ensure `gofmt -s`, `go vet`, the full test
  suite, and a short fuzz smoke all pass.
- Create the public repo: `gh repo create freeeve/libcodex --public`
  (confirm with the user immediately before the network push — outward-facing).
- Semantic initial commit, e.g.
  `feat: MARC 21 codex with iso2709, marcxml, marc-in-json and mrk codecs`.
  Per global guidelines: semantic message, functional/technical description only,
  no AI-to-owner phrasing.
- GitHub Actions CI: build, `go vet`, `gofmt -s -l` gate, `go test -race -cover`,
  and a short `-fuzz` smoke per format.
- Tag `v0.1.0`.

## Acceptance
- Public repo live at github.com/freeeve/libcodex with green CI and a semver tag;
  README renders.

## Status — done
- `git init` (main), `.gitignore`, GitHub Actions CI
  (`.github/workflows/ci.yml`: gofmt gate, vet, build, `go test -race -cover`,
  and a fuzz-smoke job that now discovers and exercises every `Fuzz*` target in
  the module, not just the original four codecs).
- Disabled setup-go dependency caching (no `go.sum` in a zero-dependency module).
- Verified clean and green: gofmt -s, go vet, `go test -race ./...`, CI run on
  the tagged commit passed both jobs.
- Semantic initial commit `b84867a`; **public repo live and `main` pushed**:
  https://github.com/freeeve/libcodex
- **Tagged `v0.1.0`** (annotated) on the green-CI commit `b5933cb` and pushed;
  GitHub Release published:
  https://github.com/freeeve/libcodex/releases/tag/v0.1.0
- Release covers the four round-trip codecs (iso2709/marcxml/marcjson/mrk) plus
  the export converters (mods, dublincore, citation, bibframe).

## Depends on
- All prior tasks. Confirm with the user before the push. [done]
