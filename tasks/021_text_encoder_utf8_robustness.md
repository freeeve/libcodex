# 021 — Invalid-UTF-8 / control-byte robustness across text encoders

## Goal
Ensure the round-trip text codecs (marcxml, marcjson, mrk) produce valid output
when converting an arbitrary decoded MARC record (MARC→format), not just when
round-tripping their own format.

## Why
The Dublin Core fuzz (`FuzzFromMARC`) feeds arbitrary MARC bytes through
`iso2709.Decode` into the encoder, and surfaced that a record claiming UTF-8
(leader byte 9 = 'a') but holding invalid UTF-8 — or a value with an XML-illegal
control byte — produces invalid output. `iso2709.Decode` trusts the bytes (the
fast path), so the encoders must guarantee valid output. mods/dublincore already
handle this; the three round-trip codecs have the same latent gap because their
fuzzers test format→model→format, not MARC→format.

## Scope
- Add a `FuzzFromMARC` target to marcxml, marcjson and mrk: `iso2709.Decode`
  arbitrary bytes → `Encode` → the output must be well-formed XML / valid JSON /
  re-readable mrk (no panic, no invalid output).
- Make each encoder robust:
  - marcxml: reject or sanitize invalid UTF-8 (round-trip format — likely extend
    `validate` to reject, consistent with its XML-illegal-char rejection).
  - marcjson: JSON requires valid UTF-8; reject or sanitize invalid bytes.
  - mrk: sanitize/reject invalid UTF-8.
- Decide reject-vs-sanitize per format (round-trip formats lean reject; lossy
  exports sanitize) and document.

## Alternative considered
Sanitize at `iso2709.Decode` (one fix, covers all) — rejected for now because it
adds a UTF-8 validation scan to the hot decode path for a malformed-input edge
case; the encoders are the right place to guarantee valid output.

## Acceptance
- `FuzzFromMARC` for all three round-trip codecs runs clean; invalid-UTF-8 and
  control-byte inputs yield valid output or a clear error, never invalid output
  or a panic.

## Depends on
- 006, 007, 008, 018
