# 008 — .mrk mnemonic (MARCMaker) format subpackage

## Goal
`mrk` subpackage: read and write the human-readable MARCMaker / MARCBreaker line
format.

## Why
`.mrk` is the editable, diffable text form catalogers use; it round-trips to
binary MARC and is handy for fixtures and code review.

## Design
- One field per line; records separated by a blank line:
  ```
  =LDR  00000nam a2200000   4500
  =001  ocm12345
  =245  10$aStone butch blues :$ba novel /$cLeslie Feinberg.
  =650  \0$aLesbians
  ```
  - `=LDR` carries the leader; two spaces then the two indicators (a blank
    indicator is shown as `\`); subfields start with `$` + code.
- Decode the MARCMaker conventions (`=LDR`, blank-indicator `\`, `$` delimiter,
  the leading `=tag  `); Encode the inverse.
- `NewReader` / `NewWriter` / `Decode` / `Encode`.
- Reuse the shared MARC-8/ANSEL helpers for any escaped mnemonics if added later
  (initial scope can be UTF-8 only).

## Performance (part of this task)
- Add `bench_test.go` (Decode/Encode/ReaderStream/WriterStream) and run the
  cross-format performance pass: append-based encoder, reuse the Writer buffer
  for ~0 allocs/record, profile, and record before/after. Keep zero third-party
  dependencies. The line format is simple, so aim near the binary codec's
  efficiency.

## Acceptance
- Round-trips with `iso2709`; handles `=LDR`, blank indicators, and repeated
  subfields; matches MARCMaker output on a golden sample; tests included.

## Status — done
- `mrk` package: `NewReader`/`NewWriter`/`Decode`/`Encode`/`ReadFile`/`WriteFile`/
  `Reader.All` — same surface contract as the other formats. No `Close()` needed
  (records self-delimit with a blank line).
- `=LDR`/`=TAG  ` lines, `\` for blank indicators, `$code` subfields, blank-line
  record separators. Escaping `$`/`{`/`}` ↔ `{dollar}`/`{lcub}`/`{rcub}`; decode
  also accepts `&#xHHHH;`/`&#DDDD;` character references (encode emits UTF-8).
- Reader is tolerant: leading/extra blank lines, non-`=` comment lines, and a
  final record without a trailing blank line.
- `Encode` rejects what the line format cannot carry (a line break in any datum;
  a `$` used as a subfield code) — `TestEncodeRejectsLineBreaks`.
- Tests: round-trip, **iso2709 ↔ mrk byte-stable**, **marcxml ↔ mrk model-equal**,
  format assertions, char references, blank indicators, reader tolerance, file
  I/O, golden (`testdata/sample.mrk`), `FuzzDecode`. Coverage 96.1%.

## Performance (done)
| Benchmark   | allocs | ns/op | MB/s |
|-------------|--------|-------|------|
| Encode      | 7      | 961   | —    |
| Decode      | 35     | 1898  | 200  |
| WriterStream (100) | 8 (~0/rec) | 79k | 480 |
| ReaderStream (100) | 3304 (~33/rec) | 154k | 248 |

The line format is parsed by hand (no `encoding/*` reflection), so it is the
fastest of the text formats — Decode is ~35 allocs vs 374 (marcxml) / 566
(marcjson). Encode is append-based; the Writer reuses its buffer (~0/record).

## Depends on
- 002 (shares MARC-8/ANSEL helpers from core; mrk is UTF-8 only for now)
