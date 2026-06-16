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

## Status — done
- Added `FuzzFromMARC` to marcxml, marcjson and mrk (iso2709.Decode arbitrary
  bytes → Encode → the output must re-decode). Campaigns clean: marcxml 6.8M,
  marcjson 5.7M, mrk 5.7M execs.
- **marcxml:** `validate` now rejects invalid UTF-8 (`xmlText` requires
  `utf8.ValidString`); indicator/subfield-code bytes must be printable ASCII
  (`xmlChar`); and the **tag** is validated for attribute-safety (`validTag`) — it
  is written unescaped, so a tag containing `"`/`<`/`&` (found by the fuzz) would
  have broken the attribute.
- **marcjson:** added `validate` rejecting invalid UTF-8 (JSON strings must be
  valid UTF-8); wired into Encode and Writer.Write.
- **mrk:** no change needed — it is byte-transparent (arbitrary non-structural
  bytes round-trip), confirmed by the fuzz.
- Round-trip codecs reject the unrepresentable (consistent with their existing
  delimiter / XML-illegal / line-break rejections); the lossy exports
  (mods/dublincore) sanitize instead. `iso2709.Decode` stays fast (no UTF-8 scan).

## Acceptance — met
- All three round-trip codecs' `FuzzFromMARC` run clean; invalid-UTF-8 and unsafe
  inputs yield a clear error, never invalid output or a panic. Coverage marcxml
  92.2%, marcjson 87.9%, mrk 96.1%.

## Depends on
- 006, 007, 008, 018
