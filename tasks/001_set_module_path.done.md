# 001 — Set module path to github.com/freeeve/libcodex

## Goal
Replace the placeholder module path `libcodex` with the canonical
`github.com/freeeve/libcodex` so the module is `go get`-able and the per-format
subpackage import paths resolve.

## Why
`go get libcodex` / `import "libcodex"` do not resolve for external users. The
path must be set before the format subpackages exist (task 002), because their
import paths derive from it.

## Steps
- `go.mod`: `module github.com/freeeve/libcodex`.
- Update README install/import examples to the real path.
- Keep the package identifier `codex` (import path != package name is fine —
  document the one-line note that import is `.../libcodex` but the package is
  `codex`).
- `go build ./... && go test ./...` stay green.

## Acceptance
- Module path is `github.com/freeeve/libcodex`; build and tests pass; README
  examples use the real path.

## Depends on
- None — do first.
