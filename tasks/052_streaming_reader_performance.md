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

- [ ] Profiles captured and summarized in the task file.
- [ ] marcjson ReaderStream: >=2x throughput or a documented decline
      rationale.
- [ ] marcxml ReaderStream: >=1.5x or documented decline rationale.
- [ ] Zero behavior change: conformance + fuzz suites pass; golden files
      byte-identical.
- [ ] benchstat before/after committed with the change.
