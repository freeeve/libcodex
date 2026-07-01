# 044 -- internal: deduplicate codec and crosswalk helpers

## Motivation

The review found the same helpers reimplemented in up to five packages.
Divergence is not hypothetical: marcjson's validation already drifted from
marcxml's (task 043), and unimarc's `joinSub` silently differs from the
other three (no `trimISBD`). Every fix in tasks 042/043/050 must currently
be applied in several places.

## Duplication inventory

Codec plumbing (three near-byte-identical copies each):

- `ReadFile`/`WriteFile`/`Decode` -- marcxml.go:406-425/502-519,
  marcjson.go:441-460/543-560, mrk.go:298-317/348-361.
- `indByte`/`codeByte` -- marcxml.go:206-219, marcjson.go:110-123 (mrk has
  its own variants).
- Test helpers `errWriter`/`selfConsistent`/`readAll` triplicated across the
  three test files.
- The `Close() error` type assertion on writers is repeated at five call
  sites (integration_test.go:63, conformance_test.go:38, export_test.go:53,
  interop_test.go:344, plus realdata usage) -- and library consumers must
  rediscover the pattern themselves.

Crosswalk helpers (export packages):

- `trimISBD` x4 -- mods.go:388-394, dublincore.go:163-169,
  citation.go:172-178, schemaorg.go:220-226 (identical).
- `joinSub` x4 -- dublincore.go:106, citation.go:124, schemaorg.go:185,
  unimarc/accessors.go:110 (last one divergent).
- `year`+`isDigit` x2 -- citation.go:161-170, schemaorg.go:209-218.
- Subject `a/x/y/z/v`-join-with-`--` builder x3 -- dublincore.go:118,
  citation.go:136, schemaorg.go:172.
- Byte-identical JSON string escapers x2 -- dublincore.go:290,
  schemaorg/json.go:118.
- `writeAll`/`WriteFile` boilerplate x4 -- mods.go:467-495,
  dublincore.go:387/451/461, schemaorg/json.go:215-243.

## Change

- Export `codex.Close(w RecordWriter) error` (or a named
  `RecordWriteCloser` interface), reference it from `Convert`'s docs, and
  replace the five duplicated assertions.
- Hoist generic `ReadFile`/`WriteFile` (and optionally `DecodeOne`) into
  `codex`, written against `RecordReader`/`RecordWriter`.
- Create `internal/crosswalk` holding `trimISBD`, `joinSub`, `year`, the
  subject joiner, and the JSON string escaper; each export package keeps
  only its format-specific mapping. Reconcile unimarc's `joinSub`
  divergence deliberately (decide whether it *should* trim ISBD).
- Put indicator/code/tag validation-and-normalization helpers in one shared
  spot so tasks 042/043 land once (sequence this task with them).

## Acceptance

- [ ] No copy of `trimISBD`/`joinSub`/JSON escaper remains outside the
      shared package (grep-clean).
- [ ] marcxml/marcjson/mrk `ReadFile`/`WriteFile` are one implementation.
- [ ] `codex.Close` exists, documented from `Convert`; test call sites
      updated and Close errors checked.
- [ ] Public API of every existing package unchanged (internal refactor
      only, plus the one `codex.Close` addition).
