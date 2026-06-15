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

## Status — done
- `dublincore` package: `FromRecord(*codex.Record) *DC` (the 15 elements),
  `Encode` (oai_dc XML), `EncodeJSON` (DC-in-JSON), `Writer` (oai_dc XML
  collection) and `JSONWriter` (JSON array) — both implement `codex.RecordWriter`
  for `codex.Convert` — and `WriteFile`.
- MARC→DC crosswalk (single pass): 245→title, 1xx→creator, 7xx→contributor,
  6xx→subject (--joined), 5xx→description, 260/264$b→publisher, 260/264$c→date,
  leader/06→type (DCMI Type), 300→format, 020/022/024/856→identifier,
  008/041→language, 506/540→rights. Lossy and one-way (DC→MARC out of scope).
- Hand-rolled XML and JSON output (no `encoding/*` reflection), UTF-8-aware
  escaping. Golden files (`sample.oai_dc.xml`, `sample.dc.json`).

## Performance (done)
| Benchmark | allocs | ns/op |
|-----------|--------|-------|
| Encode (XML)  | **20** | 902 |
| EncodeJSON    | **23** | 1189 |
| WriterStream (100) | 1609 (~16/rec) | 709 MB/s |

Hand-rolled flat output is far leaner than reflection-based marshaling.

## Fuzz (done)
`FuzzFromMARC`: any decodable MARC record converts to **well-formed XML and valid
JSON** without panicking. Campaign clean (12.5M execs).
- **Finding fixed:** values from malformed records can contain XML-illegal control
  bytes or invalid UTF-8; the hand-rolled escapers now drop them (lossy export),
  so output is always valid. NOTE: the same latent gap exists in the round-trip
  text encoders (marcxml/marcjson/mrk) on the MARC→format path — tracked in 021.

## Acceptance — met
- MARC→DC crosswalk documented and implemented; oai_dc XML + DC-JSON output;
  golden-file tests; coverage 82.7%.

## Depends on
- 002

## Per-format requirements (standing directive)
- Add `bench_test.go`; profile and reduce allocations (single-pass mapping; reuse
  the Writer buffer; hand-roll output where it pays off). Document any stdlib
  marshaling floor.
- Add a fuzz target: any decodable MARC record converts without panicking and
  produces valid output. Run a sustained campaign.
- Zero third-party dependencies.
