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

## Depends on
- All prior tasks. Confirm with the user before the push.
