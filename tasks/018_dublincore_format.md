# 018 — Dublin Core (mapping layer)

## Goal
Export records to Dublin Core (DCMI) — the lowest-common-denominator metadata
used by OAI-PMH and most repository software (DSpace, Fedora, Samvera).

## Why
Dublin Core is the most widely consumed bibliographic metadata format on the web
and the default for harvesting. A MARC→DC crosswalk (LoC publishes one) reaches a
huge audience, at the cost of being lossy (15 flat elements).

## Scope
- `dublincore` package; emit both **simple DC** as `oai_dc` XML and a DC-in-JSON
  form. `encoding/xml` / `encoding/json` only.
- MARC→DC crosswalk for the 15 elements (title←245, creator←100/110/111,
  subject←6xx, publisher←260/264$b, date←260/264$c or 008, type←leader/006,
  identifier←020/022/856, language←008/041, description←5xx, etc.).
- A converter API (`dublincore.FromRecord(*codex.Record) DC`), not a
  `RecordReader`/`RecordWriter` (DC is a different, lossy model). DC→MARC is out
  of scope (too lossy to be useful).

## Acceptance
- Documented MARC→DC crosswalk; `oai_dc` XML output is schema-valid; golden-file
  tests over a representative corpus.

## Depends on
- 002

## Per-format requirements (standing directive)
- Add `bench_test.go`; profile and reduce allocations (single-pass mapping; reuse
  the Writer buffer; hand-roll output where it pays off). Document any stdlib
  marshaling floor.
- Add a fuzz target: any decodable MARC record converts without panicking and
  produces valid output. Run a sustained campaign.
- Zero third-party dependencies.
