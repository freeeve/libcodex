# 015 — BIBFRAME mapping (done)

## Goal
Add BIBFRAME 2.0 support: convert `codex.Record` (MARC) to BIBFRAME RDF
(Work / Instance resources).

## Resolved decisions
- **Direction:** MARC→BIBFRAME only. The reverse (BIBFRAME→MARC) is lossy and
  out of scope; BIBFRAME is documented as export-only, like MODS/DC/citation.
- **Serializations:** RDF/XML (the canonical LoC form) and JSON-LD. Both are
  hand-written.
- **Dependency policy:** stdlib only — no RDF library. The serializers are
  append-based emitters (the same approach as `dublincore`/`citation`), so the
  project's zero-dependency rule holds. JSON-LD uses a fixed `@context` prefix
  map and an `@graph` of two nodes; RDF/XML uses striped syntax with relative
  fragment IRIs minted from the 001 control number (or a per-record index).
- **Vocabulary scope:** the common-field crosswalk —
  - Work: content class (leader/06), titles (245 transcribed; 130/240 uniform
    preferred), contributions (1xx primary / 7xx added, agent class by tag,
    role from $e/$4), subjects (650 Topic, 651 Place, 6x0 names), genre (655),
    languages (008/35-37 + 041, validated to ISO 639-2), classification
    (050 LCC, 082 DDC), summary (520).
  - Instance: transcribed title, responsibility (245$c), edition (250),
    provision/publication (260/264 place/agent/date), extent (300), identifiers
    (020 ISBN / 022 ISSN / 024), electronic locator (856$u).

## Delivered
- `bibframe` package: `bibframe.go` (model + crosswalk + writers),
  `rdfxml.go` (RDF/XML emitter), `jsonld.go` (JSON-LD emitter).
- Surface: `FromRecord`, `Encode` / `EncodeJSONLD`, streaming `Writer` /
  `JSONLDWriter` (implement `codex.RecordWriter`, need `Close`),
  `WriteFile` / `WriteJSONLDFile`.
- Tests: crosswalk assertions, XML well-formedness + JSON validity (parsed back
  with the stdlib tokenizers), escaping/edge cases, writer error paths, golden
  files. Coverage ~97%.
- Benchmarks (`bench_test.go`); allocation-tuned (slice dedup for languages,
  pre-sized encode buffers, buffer-reused streaming writers).
- Fuzz `FuzzFromMARC` (arbitrary MARC → both serializations): 12.9M execs clean
  after fixing one real bug — an unvalidated language code from a malformed
  008/041 was emitted unescaped into a vocabulary IRI; now restricted to
  `[a-z]{3}`.
- Added to the cross-package `TestExportConvertersCanonical` smoke test over the
  real MARC-8 corpus, and documented in the README.

## Acceptance — met
- Direction + serialization + dependency policy decided and documented (here +
  package doc + README).
- A `bibframe` package converting `*codex.Record` → BIBFRAME in the chosen
  serializations, with a documented, tested mapping for the common fields.
