# 007 — MARC-in-JSON format subpackage

## Goal
`marcjson` subpackage: read and write the de-facto "MARC-in-JSON" structure.

## Why
MARC-in-JSON (the Ross Singer / pymarc-compatible shape) is the common JSON
representation of MARC; same data model as `codex.Record`.

## Design
- `encoding/json` only.
- Shape:
  ```json
  {
    "leader": "00000nam a2200000   4500",
    "fields": [
      {"001": "ocm12345"},
      {"245": {
        "ind1": "1", "ind2": "0",
        "subfields": [{"a": "Stone butch blues :"}, {"c": "Leslie Feinberg."}]
      }}
    ]
  }
  ```
- `NewReader` decodes a stream/array of records or one object; `NewWriter`
  encodes. `Decode` / `Encode` single-record helpers.
- Preserve order: `fields` is an ordered array and each `subfields` entry is
  ordered (a control field is a single `{tag: value}` object).

## Performance (part of this task)
- Add `bench_test.go` (Decode/Encode/ReaderStream/WriterStream) and run the
  cross-format performance pass: hand-roll the encoder if `encoding/json`
  reflection dominates, reuse the Writer buffer for ~0 allocs/record, profile,
  and record before/after. Keep zero third-party dependencies.

## Status — done
- `marcjson` package: `NewReader`/`NewWriter`/`Decode`/`Encode`/`ReadFile`/
  `WriteFile`/`Reader.All` — same surface contract as the other formats.
- Layout matches pymarc/ruby-marc/marc4j: `{"leader","fields":[…]}`, control
  fields `{tag: string}`, data fields `{tag:{ind1,ind2,subfields:[{code:value}]}}`.
- Reader accepts a single object, a whitespace-separated object stream, or a
  top-level array; tolerates and skips unknown keys. Writer emits a JSON array
  and requires `Close()` (consistent with marcxml).
- Hand-rolled JSON string escaping (`\"` `\\` `\n` `\r` `\t`, `\u00XX` for other
  control chars; `&`/`<` left raw, valid in JSON). UTF-8 preserved.
- `encoding/json` only. Tests: round-trip, **iso2709 ↔ marcjson byte-stable**,
  **marcxml ↔ marcjson model-equal**, input shapes, escaping, unknown keys, 16
  malformed-input error cases, file I/O, golden (`testdata/sample.json`),
  `FuzzDecode`. Coverage 89.2%.

## Performance (done)
| Benchmark   | allocs | ns/op | notes |
|-------------|--------|-------|-------|
| Encode      | **7**  | 1421  | hand-rolled appender |
| WriterStream (100) | **10** (~0.1/rec) | 95k | + buffer reuse; 817 MB/s |
| Decode      | 566    | 19221 | `json.Decoder.Token()`-bound |
| ReaderStream (100) | 55909 | 1.9ms | `json.Decoder.Token()`-bound |

- Encode/write hand-rolled + buffer-reused → ~0 allocs/record streaming.
- Decode walks `json.Decoder.Token()` (no reflection, order-faithful, handles
  all input shapes). Its cost is the stdlib token API: each token is boxed into
  `interface{}` and string values are un-escaped rune-by-rune (profile:
  `Token`/`literalStore`/`utf8.AppendRune`). Lower would need restricting input
  to arrays (to use `json.Unmarshal`) or a hand-rolled JSON parser — neither
  worth the trade-off for a non-hot text path. Documented, not taken.

## Acceptance — met
- Round-trips with `iso2709` (byte-stable) and `marcxml` (model-equal); matches
  the pymarc-style layout; golden-file test included.

## Depends on
- 002
