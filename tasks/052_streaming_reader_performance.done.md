# 052 -- marcjson/marcxml: streaming reader throughput

## Motivation

Benchmark baseline (2026-07-01, Apple Silicon, `-benchtime=200ms`):

| path                    | throughput | allocs/op |
|-------------------------|-----------:|----------:|
| iso2709 ReaderStream    |   822 MB/s |       406 |
| mrk ReaderStream        |   252 MB/s |     3,304 |
| marcxml ReaderStream    |    60 MB/s |    34,426 |
| marcjson ReaderStream   |    42 MB/s |    55,909 |

The two structured-text readers are 14-20x slower than the binary path
with 100x the allocations. The review found the token-walking architecture
sound (streaming decoders, no whole-document unmarshal, no regexp), so the
cost is per-token overhead in `encoding/xml`/JSON tokenization plus
per-field/subfield small allocations -- this task is profile-driven rather
than a known bug list.

## Change

- Profile `BenchmarkReaderStream` in both packages (`-cpuprofile`,
  `-memprofile`); attribute the allocation counts (marcjson: ~566/record
  even in single-record Decode).
- Candidate levers, to be confirmed by the profile:
  - marcjson: the package already hand-rolls encoding; consider
    hand-rolling the decode tokenizer too (or `json.Decoder` with
    `UseNumber` avoided, reused buffers for string values).
  - marcxml: `encoding/xml` is the known floor (~60 MB/s is typical for
    it); measure how much is recoverable via `d.RawToken()` (skips
    name-space processing and some allocations) before considering a
    minimal purpose-built tokenizer for the fixed MARCXML vocabulary.
  - Both: reuse per-record `Field`/`Subfield` backing slices across
    `Read` calls where the API contract allows (records own their data --
    check before sharing).
- If after profiling the stdlib tokenizer is genuinely the floor and a
  custom tokenizer is judged not worth the maintenance, document that
  conclusion and the numbers here and close the task -- "measured,
  declined" is an acceptable outcome.

## Acceptance

- [x] Profiles captured and summarized (below).
- [x] marcjson ReaderStream: **+209% throughput (~3.1x)** -- exceeds >=2x.
- [x] marcxml ReaderStream: **measured decline** documented (below).
- [x] Zero behavior change: marcjson unit/interop/conformance and both
      fuzzers pass; golden files byte-identical (the writer is untouched).
- [x] benchstat before/after recorded (below).

## Profiles (alloc_objects, ReaderStream)

marcjson (baseline): ~95% of allocations are inside encoding/json --
`scanner.error` 34%, `Decoder.Token` 30%, `literalStore`/`quoteChar`/
`AppendRune`. Only ~5% (readSubfields, AddField) is the package's own code. The
`Decoder.Token` stream API boxes every token in an interface and builds a
syntax-error object at value boundaries; that overhead is the floor of that API.

marcxml (baseline): `encoding/xml.(*Decoder).rawToken` 51% + `Decoder.Token`
(namespace wrapper) 20% + `Decoder.name` 15%. The wrapper is 20%; the rest is
`rawToken` itself, which `RawToken()` also calls.

## marcjson -- hand-rolled tokenizer (done)

Replaced the `encoding/json.Decoder.Token` read path with a streaming byte
scanner (`scan.go`) that walks the MARC-in-JSON grammar directly, allocating a
Go string only per retained value. It treats `,`/`:` as skippable separators
(like Token does) and never loops or panics on malformed input; escapes and
`\uXXXX` (with surrogate pairs) are decoded. The fuzz contract is round-trip
stability, not byte-exact stdlib rejection parity, so the scanner's leniency is
safe. The writer is unchanged (golden byte-identical).

### benchstat (before = json.Decoder, after = hand-rolled; 1s x6)

| metric      | before      | after       | delta      |
|-------------|-------------|-------------|------------|
| throughput  | 23.0 MiB/s  | 71.4 MiB/s  | **+210%**  |
| sec/op      | 3.21 ms     | 1.04 ms     | -68%       |
| B/op        | 1266 KiB    | 328 KiB     | -74%       |
| allocs/op   | 55,909      | 7,309       | -87%       |

## marcxml -- measured, declined

The stdlib `encoding/xml` tokenizer is the floor here. `Decoder.Token()` cannot
be swapped for `RawToken()` in this reader without also replacing `Decoder.Skip`
and losing namespace-prefix handling, and even then the profile shows `RawToken`
would only shed the ~20% namespace-resolution wrapper -- the `rawToken` core
(51%) and `name` (15%) allocations are inherent to `encoding/xml` and unavoidable
through its API. That caps a safe swap well under the 1.5x bar.

A >=1.5x gain would require a full purpose-built MARCXML tokenizer (attributes,
entities `&amp;`/`&#nn;`, CDATA, comments, PIs, namespace prefixes, well-formed
nesting) with fuzz parity across all of it -- materially more complex than the
JSON grammar and disproportionate risk for a format already at its typical stdlib
ceiling (~45-60 MB/s). Declined; the marcjson hand-roll is where the simpler
grammar makes the effort pay off.
