# 012 — Thorough fuzz pass: mrk

## Goal
Harden the `mrk` mnemonic codec with a thorough fuzz pass.

## Scope
- `FuzzDecode`: arbitrary/line-shaped text never panics; malformed lines are
  skipped or errored, not crashed.
- `FuzzEncodeRoundTrip`: a record built from fuzzed model values encodes to
  MARCMaker text that decodes back equal (`Encode → Decode` is identity on the
  model), with attention to the `$` delimiter, blank-indicator `\`, embedded
  `$`/newlines in values, and the `=LDR` line.
- `FuzzRoundTrip`: `Decode → Encode → Decode` is stable for inputs that decode
  cleanly.
- Seed corpus: valid multi-record `.mrk`, missing `=LDR`, blank indicators,
  repeated subfields, blank-line separators, empty input.
- Sustained campaign; commit crashers under `mrk/testdata/fuzz`.

## Status — done
- `FuzzDecode`: arbitrary input never panics; `Decode→Encode→Decode` is stable
  for self-consistent records (skipping records `Encode` rejects). Campaign clean
  (~4.5M execs); crasher seed committed under `mrk/testdata/fuzz/FuzzDecode`.
- **Finding fixed:** a line break (`\r`/`\n`) in an *indicator* — or a `$` used as
  a *subfield code* — was not rejected by the initial `validate` and corrupted the
  round trip (a reader strips a trailing CR as a line ending). `Encode` now
  rejects both; covered by `TestEncodeRejectsLineBreaks`.
- Uses the same `selfConsistent` guard as the other text formats for the
  tag/element-classification edge.

## Acceptance
- No panics or hangs; round-trip invariant holds for representable values;
  delimiter/indicator edge cases handled; seed corpus committed.

## Depends on
- 008
