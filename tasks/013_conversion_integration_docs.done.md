# 013 — Cross-format conversion, integration tests & docs

## Goal
Tie the formats together and document the multi-format library.

## Scope
- Cross-format round-trip integration tests: `iso2709 ↔ marcxml ↔ marcjson ↔ mrk`
  all preserve the model (a matrix test over a shared record corpus).
- A small example that stream-converts one format to another purely through the
  `codex.RecordReader` / `codex.RecordWriter` interfaces — this is the proof that
  the extensibility seam works.
- README rewrite: architecture (core model + interfaces + format subpackages),
  per-format usage, conformance/scope notes, performance notes, and a
  "how to add your own format" section.
- Package-level doc comments for each subpackage.

## Acceptance
- Conversion matrix test green; README documents all four formats plus the
  extension point; runnable `Example` functions compile under `go test`.

## Status — done
- `codex.Convert(RecordReader, RecordWriter) error` added — the whole conversion
  engine, written against the interfaces; unit-tested for read/write errors.
- `integration_test.go` (`package codex_test`): `TestAllFormatsPreserveModel`
  (every format round-trips the corpus) and `TestConversionMatrix` (all **16**
  source→target combinations via `codex.Convert` preserve the model). The corpus
  is normalized through iso2709 once so leaders carry their computed length/base.
- `ExampleConvert` — runnable, output-verified: binary ISO 2709 → MARCMaker via
  the interfaces.
- README rewritten for the multi-format library: architecture + format table,
  uniform reading/writing, `codex.Convert`, accessors, a performance table, the
  MARC-8 scope, per-format "what each format rejects", the gzip compose-via-`io`
  note, and a "how to add your own format" section. Each subpackage already has a
  package doc comment.
- The optional `.gz` file-helper auto-detection was offered but not added;
  compression composes through `io` (documented), keeping the codecs orthogonal.

## Acceptance — met
- Conversion matrix green (16/16 + 4 round-trips); README documents all four
  formats and the extension point; the `Example` runs under `go test`. Core
  coverage 100%.

## Depends on
- 002, 006, 007, 008
