# 006 — MARCXML format subpackage

## Goal
`marcxml` subpackage: read and write the LoC MARCXML serialization of the MARC
model.

## Why
MARCXML (LoC "slim" schema, namespace `http://www.loc.gov/MARC21/slim`) is a
primary interchange format alongside binary MARC. Same data model, so it maps
directly onto `codex.Record`.

## Design
- `encoding/xml` only (no third-party deps).
- `marcxml.NewReader(io.Reader)` streams `<record>` elements out of a
  `<collection>` (or a bare `<record>`), implementing `codex.RecordReader`.
- `marcxml.NewWriter(io.Writer)` emits a `<collection>` wrapper containing:
  ```xml
  <record>
    <leader>00000nam a2200000   4500</leader>
    <controlfield tag="001">ocm12345</controlfield>
    <datafield tag="245" ind1="1" ind2="0">
      <subfield code="a">Stone butch blues :</subfield>
      <subfield code="c">Leslie Feinberg.</subfield>
    </datafield>
  </record>
  ```
- `Decode([]byte)` / `Encode(*codex.Record)` single-record helpers.
- XML-escape values; preserve field and subfield order; indicators as
  single-character attributes (blank → a space).

## Status — done
- `marcxml` package: `NewReader`/`NewWriter`/`Decode`/`Encode`/`ReadFile`/
  `WriteFile`/`Reader.All` — the same surface contract as `iso2709`.
- Reader streams `<record>` elements via `xml.Decoder` (namespace-agnostic:
  handles the slim namespace, prefixed namespaces, and bare records with no
  `<collection>`).
- Writer emits `<?xml?>` + `<collection xmlns="…/slim">` and requires `Close()`
  to emit `</collection>` (standard streaming pattern; the `RecordWriter`
  interface has no finalizer). `Encode` produces a standalone namespaced
  `<record>`.
- `encoding/xml` only; values XML-escaped; UTF-8 preserved; blank indicators
  emitted as spaces; control-then-data order per the slim schema.
- Tests: encode/decode round-trip, **iso2709 ↔ marcxml byte-stable cross-format**,
  escaping + namespace, namespace variants (prefixed/bare/no-ns), missing-attr
  decode, Writer error/idempotency, file I/O, and a golden collection
  (`testdata/sample.xml`, regenerate with `UPDATE_GOLDEN=1`). Coverage 92.1%.

## Acceptance — met
- Round-trip `iso2709 → marcxml → iso2709` is byte-stable; output is slim-schema
  shaped (collection + namespace); golden-file test included.

## Performance (done)
Benchmarks added (`bench_test.go`); profiled with pprof.

| Benchmark   | allocs before→after | ns before→after | notes |
|-------------|---------------------|-----------------|-------|
| Encode      | 139 → **9**         | 10705 → 995 (10.8×) | hand-rolled encoder |
| WriterStream (100) | 13803 → **13** (~0.13/rec) | 1129k → 63k (17.8×) | + buffer reuse; 2235 MB/s |
| Decode      | 472 → **374**       | 31k → 24k (1.3×) | token-walking, stdlib |
| ReaderStream (100) | 45926 → **36326** | 3.1ms → 2.4ms (1.3×) | token-walking, stdlib |

- **Encode/write rewritten** to append XML directly to a buffer (own
  chardata/attr escaping, with `\r` → `&#xD;` so values survive XML line-ending
  normalization), replacing reflection-based `xml.MarshalIndent`. The Writer
  reuses one buffer, so streaming writes are ~0 allocs/record.
- **Decode stays on `encoding/xml` but drops reflection.** Profiling showed ~25%
  of decode allocations were the reflection layer (`unmarshal`/`copyValue`/
  `reflect.growslice`/`unmarshalAttr`); switching from `DecodeElement` to manual
  `xml.Decoder.Token()` walking removed them — 470 → 374 allocs, ~1.3× faster —
  while staying zero-dependency and gaining exact field-order fidelity. The
  remaining ~68% is the stdlib tokenizer (`rawToken`/`Token`/`name`), the floor
  without replacing the parser. A faster third-party tokenizer (e.g.
  tdewolff/parse) could go lower but would break the zero-dependency design; left
  as an explicit future trade-off, not taken.

## Depends on
- 002
