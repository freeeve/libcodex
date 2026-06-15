# 011 — Thorough fuzz pass: marcjson

## Goal
Harden the `marcjson` codec with a thorough fuzz pass.

## Scope
- `FuzzDecode`: arbitrary/JSON-shaped bytes never panic; malformed JSON returns
  an error rather than crashing.
- `FuzzEncodeRoundTrip`: a record built from fuzzed model values encodes to valid
  JSON that decodes back equal (`Encode → Decode` is identity on the model),
  with attention to field/subfield ordering and Unicode escaping.
- `FuzzRoundTrip`: `Decode → Encode → Decode` is stable for inputs that decode
  cleanly.
- Seed corpus: valid records, control-only records, empty `fields`, duplicate
  keys, wrong types, deeply nested arrays, empty input.
- Sustained campaign; commit crashers under `marcjson/testdata/fuzz`.

## Status — done
- `FuzzDecode`: arbitrary input never panics; `Decode→Encode→Decode` is stable
  for self-consistent records (JSON represents every character, so no encode
  rejection is needed — the marcxml NUL issue does not apply here). Campaign
  clean (~2M execs).
- 16 malformed-input error cases (bad/missing values, wrong types, truncation at
  several points) assert clean errors rather than panics.
- Same `selfConsistent` guard as marcxml scopes the stability assertion away from
  malformed tag/element mismatches.

## Acceptance
- No panics or hangs; encoder always emits valid JSON; ordering preserved across
  round-trips; seed corpus committed.

## Depends on
- 007
