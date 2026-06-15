# 016 — MARC-8 encoding (UTF-8 → MARC-8)

## Goal
Add the MARC-8 *write* path so the library can emit legacy MARC-8 `.mrc`, not
just decode it. This completes the historical-MARC round trip: the codec already
reads MARC-8, but always writes UTF-8.

## Why
Many older catalog systems still require MARC-8. We decode MARC-8 → UTF-8 for the
Western (ASCII + ANSEL Extended Latin + combining) subset; the inverse lets us
round-trip back into MARC-8.

## Scope
- `internal/marc8.Encode(s string) ([]byte, error)` — UTF-8 → MARC-8 for the
  supported subset, the inverse of `Decode`:
  - ASCII passes through; ANSEL graphic characters map to their G1 byte.
  - Precomposed Latin characters decompose to base + combining mark, and the mark
    is emitted **before** the base (MARC-8 order, the reverse of Unicode), using
    the inverse of the existing NFC composition table.
  - A base followed by combining marks (NFD input) is reordered the same way.
  - Characters outside the subset (Greek, Cyrillic, CJK, …) → an error, so
    callers learn the record is not representable rather than getting mojibake.
- `iso2709` MARC-8 output: a way to encode a record with MARC-8 values and leader
  byte 9 = blank (e.g. `EncodeMARC8` and a Writer option), returning an error if
  any value is out of the subset.
- Property test: for every record built from the supported repertoire,
  `marc8.Decode(marc8.Encode(s)) == s`, and `iso2709` MARC-8 ↔ UTF-8 round-trips.

## Stretch (separate effort)
Full MARC-8 repertoire (Greek, Cyrillic, Hebrew, Arabic, EACC/CJK,
sub/superscripts) for both decode and encode — large code tables; out of scope
for the first pass, which targets the Western subset that covers most records.

## Status — done (Western subset)
- `internal/marc8.Encode(string) ([]byte, error)`: inverse tables derived from the
  decode tables; decomposes precomposed Latin via the NFC table and emits the
  combining mark **before** the base (MARC-8 order); reorders NFD sequences the
  same way; errors on out-of-subset runes and on the reserved control bytes
  (escape `0x1b`, separators `0x1d/1e/1f`).
- `iso2709.EncodeMARC8(*codex.Record) ([]byte, error)`: transcodes every value to
  MARC-8 and sets leader byte 9 = blank; errors if any value is out of subset.
- Tests: `marc8.Decode(marc8.Encode(s)) == s` over the repertoire; combining-mark
  reordering asserted at the byte level; out-of-subset + structural-byte
  rejection; `iso2709` MARC-8 round-trip (UTF-8 record → EncodeMARC8 → Decode
  back to UTF-8). `FuzzEncode` (Encode-after-Decode idempotence).
- **Finding fixed:** the fuzz caught the escape byte `0x1b` passing through as
  data (a reader would read it as an escape sequence); `Encode` now rejects the
  structural control bytes.
- Coverage: marc8 96.3%, iso2709 97.4%. README updated.

## Stretch (separate effort, not done)
Full MARC-8 repertoire (Greek, Cyrillic, Hebrew, Arabic, EACC/CJK,
sub/superscripts) for decode and encode — large code tables; deferred.

## Acceptance — met
- `marc8.Encode` round-trips with `Decode`; errors cleanly on unrepresentable
  input; `iso2709` writes MARC-8; fuzz + tests green.

## Depends on
- 002, 003
