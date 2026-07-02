# 074 -- sru: SRU searchRetrieve client -> codex.Record

New capability (not from the m2b audit). First network-facing package.
Ref: SRU 1.1/1.2 (LoC, https://www.loc.gov/standards/sru/); CQL query language.

## Motivation

libcodex could read/write/convert MARC in many serializations but had no way to
*fetch* records from a remote catalog. SRU is the HTTP search protocol library
systems expose for copy-cataloging and discovery (the web successor to Z39.50); it
returns bibliographic records embedded in XML, most often MARCXML -- exactly what
this library already decodes. A thin SRU client turns libcodex into a working
copy-cataloging tool: search a catalog, stream the hits into the existing
MARC <-> MARCXML <-> MODS <-> BIBFRAME <-> DC pipeline.

## Scope

1. New `sru` package (own package, so the core format packages stay network-free).
2. `Client` + `SearchRetrieve(ctx, Request) (*Response, error)` -- one page: build
   the URL, GET, parse the `searchRetrieveResponse`, extract records + diagnostics.
3. `Reader` implementing `codex.RecordReader` -- auto-pages the result set and
   decodes MARCXML via `marcxml.Decode`, so it is a `codex.Convert` source.
4. MARCXML decodes to `*codex.Record`; other schemas (MODS/DC) are exposed as raw
   XML in `Record.Data` (those crosswalks are encode-only -- decoding is out of
   scope, noted for a future task).
5. Pure stdlib (`net/http`, `net/url`, `context`, `encoding/xml`); establish the
   `httptest` fixture pattern (first network-facing tests in the repo).

## Hazards

- Sample/goldens: none affected -- new package, no shared fixtures.
- `recordPacking`: handle both `xml` (inline markup) and `string` (XML-escaped
  text -> unescape).
- Namespaces: SRU servers use `zs:`/`srw:` prefixes inconsistently -> parse by
  local name (namespace-agnostic), matching what `marcxml` decode already does.
- `recordSchema` identifiers vary (short name, `info:` URI, namespace URI) ->
  normalize to a canonical token.
- SRU 2.0 renames params (`recordXMLEscaping`) -> target 1.1/1.2, keep `Version`
  configurable.
- No live network in tests.

## Acceptance

- [x] `SearchRetrieve` parses counts/records/diagnostics; MARCXML `Record.Decode()`
      -> `*codex.Record`.
- [x] `Reader` auto-pages, satisfies `codex.RecordReader`, ends on `io.EOF`.
- [x] Diagnostics-only response -> `*DiagnosticsError`.
- [x] Both `recordPacking` modes; schema short-name and URI forms normalize.
- [x] Pure stdlib; `httptest` fixtures; suite + fuzz green; README updated.

## Result

Landed the `sru` package -- the library's first (and only) network-facing package,
standard library only.

- `sru/sru.go`: `Client` (BaseURL + optional HTTPClient/Version/Schema/MaxRecords),
  `SearchRetrieve`, request/response/record/diagnostic types, URL building,
  namespace-agnostic envelope parsing (`,innerxml` for the payload),
  `recordPacking="string"` unescaping (`xmlUnescape` via an `xml.Decoder`), and
  `normalizeSchema` (short name / `info:` URI / namespace URI -> `marcxml`|`mods`|
  `dc`). `Record.Decode()` dispatches MARCXML to `marcxml.Decode` and errors for
  other schemas, whose payload stays available in `Record.Data`.
- `sru/reader.go`: `Reader` implementing `codex.RecordReader` (compile-time
  asserted), auto-paging on `NextRecordPosition`, skipping non-MARCXML records,
  sticky errors, `All()` iterator. `codex.Convert(client.NewReader(ctx, q), w)`
  streams a search straight into any writer.
- `sru/cql.go`: `Quote` for safe CQL term interpolation (CQL is otherwise passed
  through verbatim; a full query builder is future work).
- Errors: transport/HTTP/parse failures return `(nil, err)`; a well-formed response
  returns the `Response` plus `Response.Err()` (a `*DiagnosticsError` when there are
  diagnostics and no records).

### Tests

`sru/sru_test.go` (white-box, `httptest.Server` over `testdata/` fixtures):
`SearchRetrieve` parse + request params + MARCXML decode; two-page `Reader` paging
via `codex.ReadAll`; diagnostics -> `*DiagnosticsError` from both entry points;
`recordPacking="string"` unescape; a non-MARCXML (MODS) record exposing raw payload
+ refusing to decode + being skipped by the `Reader`; `normalizeSchema` and `Quote`
tables; end-to-end `codex.Convert` into a MARCJSON writer; `FuzzParseResponse`
(no-panic on arbitrary input). `sru/bench_test.go` benchmarks the page parse.
Fixtures in `sru/testdata/` with `SOURCES.md` provenance (synthetic, LC SRU shape).

### Deferred (future tasks)

- SRU 2.0 parameter set; `explain` and `scan` operations; a CQL query builder.
- MODS/DC -> `codex.Record` decoders (would let those schemas decode too).
- Z39.50 transport for the same use case -- see `tasks/075_z3950_client.md`.
