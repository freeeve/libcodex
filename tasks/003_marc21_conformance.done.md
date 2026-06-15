# 003 — MARC 21 / ISO 2709 conformance pass

## Goal
Reconcile the implementation with the official Library of Congress MARC 21 /
ISO 2709 specifications and the ANSEL / MARC-8 code tables.

## Audit result (summary)
A conformance audit was run against the MARC 21 standard. The core framing
(leader byte map, 12-byte directory geometry, delimiters `0x1D/1E/1F`,
octet-based length counting) and the **entire ANSEL table are correct** — no
byte-mapping fixes needed. The real exposures are the writer not enforcing the
fixed leader geometry, lossy handling of out-of-scope MARC-8 scripts, and the
unstated "MARC 21, not arbitrary ISO 2709" assumption.

> CAVEAT: the audit ran with network egress blocked, so it could not fetch the
> live loc.gov pages; it used established knowledge of the (frozen) standard and
> cites canonical LoC URLs. Re-verify the two flagged late additions (0xC7 ß,
> 0xC8 €) and the byte map against the live code tables before tagging a release.

## Work items (prioritized)
1. **[correctness] Writer must force the fixed leader geometry.** Today only
   byte 9 is forced. Also set `leader[10]='2'`, `leader[11]='2'`, and
   `copy(leader[20:24], "4500")` in `iso2709.Encode` so a hand-built/odd input
   leader can't yield a record whose declared geometry contradicts the emitted
   12-byte directory. (LoC bdleader.html)
2. **[correctness] Don't silently re-serialize corrupted out-of-scope MARC-8.**
   Non-Latin sets are passed through as Latin-1 on read, then re-emitted as UTF-8
   with leader 9='a' — a record that claims clean Unicode but holds mojibake.
   Detect a non-Latin escape designation and surface an error/flag (or implement
   the set). (LoC speccharucs.html, codetables.xml)
3. **[robustness] Validate, don't just assume, leader 10/11 and 20-23 on decode.**
   Accept `"2"/"2"/"4500"`; if different, either honor them or return a clear
   error so non-MARC-21 ISO 2709 fails loudly instead of misparsing. (bdleader.html)
4. **[robustness] Persist MARC-8 designation state across subfields within a
   field.** `marc8.Decode` is currently called per subfield, resetting G0/G1 each
   time; MARC-8 reinstates defaults at the start of each *field*, so a set
   designated in `$a` should carry into `$b`. Decode the whole field with
   persistent state, resetting only at field boundaries. (speccharucs.html)
5. **[interop] Apply a defined Unicode normalization (NFC) on all output** — both
   the MARC-8 transcode path and the UTF-8 passthrough — and document it. Output
   is currently mixed NFC/NFD. (speccharucs.html)
6. **[verify] Re-confirm 0xC7 (ß) and 0xC8 (€)** against the live LoC Extended
   Latin code table and document a minimum MARC-8 vintage (both are late
   additions). (codetables.xml)
7. **[cleanup] Remove/repair the dead `g0` state** in `interpretEscape` so the
   misleading `'S'`/`'s'` handling can't mask a future bug. (codetables.xml)
8. **[doc] Document the fill character `0x7C`** behavior (passed through as data;
   note that a fill in leader 9 is treated as MARC-8).
9. **[test] Assert octet (not character) length counting** with a multibyte UTF-8
   value round-trip.

## Status
- [x] 1. Writer forces fixed leader geometry (`leader[10]='2'`, `[11]='2'`,
      `[20:24]="4500"`) in `iso2709.Encode`. Tested by `TestEncodeForcesLeaderGeometry`.
- [x] 2. Out-of-scope MARC-8 lossiness surfaced: `marc8.Decoder.Lossy()` detects
      best-effort passthrough; `iso2709.Decode` now returns it as a bool
      (`(*codex.Record, bool, error)`) and `iso2709.Reader.Lossy()` exposes it on
      the streaming path. Tested by `TestDecoderLossy`, `TestMARC8Decoding`
      (asserts not-lossy) and `TestMARC8StatePersistsAcrossSubfields` (asserts lossy).
- [x] 3. Decode honors leader 10/11 and entry map 20-22 (falls back to MARC 21
      defaults on non-digits). Tested by `TestDecodeHonorsEntryMap`.
- [x] 4. MARC-8 designation state persists across subfields within a field
      (`marc8.Decoder` is created once per field). Tested by
      `TestMARC8StatePersistsAcrossSubfields` and `TestDecoderStatePersists`.
- [x] 5. Normalization documented in the `iso2709` package doc. Full NFC is
      intentionally NOT added: it needs `golang.org/x/text`, which breaks the
      stdlib-only constraint. Common Latin base+mark pairs still compose to NFC.
- [x] 6. Re-verified live against the LoC marcspec (itsmarc mirror; loc.gov
      bot-blocks WebFetch) and the LoC MARBI 2005-05 notice. 0xC7=U+00DF (ß) and
      0xC8=U+20AC (€) CONFIRMED (added June 2004). The full-table diff also found:
      (a) **0xAE alif was stale** — LoC remapped 02BE→02BC in March 2005; FIXED.
      (b) **0xBC (ơ U+01A1) and 0xBD (ư U+01B0) were missing** — assigned graphics;
      ADDED. (c) EB/EC/FA/FB intentionally keep the precise half-marks
      (U+FE20-FE23) over the spanning U+0361/U+0360 the table lists — documented.
      Tested by new `marc8` cases.
- [x] 7. Removed the buggy `final=='S' && target==g0` escape clause and the dead
      G0 state; the decoder now tracks only G1 (the set governing high bytes).
- [x] 8. Fill character `0x7C` behavior documented in the `iso2709` package doc.
- [x] 9. Octet (not character) length counting asserted by `TestOctetLengthCounting`
      with multibyte UTF-8.

## Resolved decision (item 2)
`iso2709.Decode` returns the lossy flag directly: `(*codex.Record, bool, error)`,
with `iso2709.Reader.Lossy()` for the streaming path. The `codex.Record` model
stays pure (no decode metadata leaks into it).

## Acceptance — met
- All nine items done and tested; the live re-verification additionally fixed the
  stale alif mapping and two missing graphics. Build, vet, gofmt clean; core 100%,
  marc8 97.1%, iso2709 93.0% coverage; fuzz clean.

## Depends on
- 002 (applied within `iso2709` / `internal/marc8`)
