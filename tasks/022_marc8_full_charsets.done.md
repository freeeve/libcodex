# 022 — Full MARC-8 character set support (done)

## Goal
Bring the previously out-of-scope MARC-8 graphic character sets into scope for
both reading and writing, replacing the best-effort Latin-1 passthrough.

## Delivered
- A G0/G1 dual-designation decoder (was G1-only): GL bytes (0x21-0x7E) decode via
  G0, GR bytes (0xA1-0xFE) via G1; escape sequences re-designate either.
- All MARC-8 sets supported, decode and encode:
  - Basic Latin (ASCII) + Extended Latin (ANSEL), with combining diacritics
    (unchanged hand-maintained tables).
  - Basic/Extended Cyrillic, Basic/Extended Arabic, Basic Hebrew, Basic Greek,
    Greek Symbols, Subscripts, Superscripts.
  - the multibyte CJK set EACC (~15,700 ideographs).
- Combining marks generalized: any decoded combining mark (Unicode Mn/Mc/Me) is
  buffered and emitted after its base (MARC-8 stores marks before the base); the
  encoder mirrors this for every set. Consecutive marks keep their order.
- `iso2709.EncodeMARC8` now transcodes across all sets, emitting ISO 2022 escapes
  and returning to the defaults at the end of each value so per-field decoders
  stay consistent. It errors only on a character no set can represent (e.g. emoji).
- Tables generated from the authoritative LoC `codetables.xml`
  (`internal/marc8/gen`, wired to `go generate ./internal/marc8`); regeneration is
  idempotent. The hand-maintained ANSEL table is retained and unchanged.

## Tests
- Round-trip tests per script; every-set decode coverage; malformed-escape and
  truncated-EACC edges. marc8 coverage ~97%.
- Real-data test over pymarc's `test_marc8.txt` (Latin/CJK/Arabic/Hebrew):
  1514/1515 lines round-trip losslessly; the one exception is an unmapped EACC
  triple, handled without a crash.
- `FuzzEncode` extended with non-Latin seeds; 53M+ executions clean after fixing
  three real stability bugs it found (leading/cross-script combining marks not
  gathered before their base; the EACC base not gathering marks; consecutive
  marks reordering). Fuzz seeds kept as regression cases.

## Notes / limitations
- Precomposed accented *non-Latin* text (e.g. NFC Greek `ά`) has no single MARC-8
  code and the standard library has no NFD normalizer, so encoding expects the
  decomposed (NFD) form — which is exactly what decoding produces. Documented in
  the README and package doc.

## Acceptance — met
- Every MARC-8 graphic set reads and writes; decode never crashes and flags lossy
  only for genuinely unrecognized designations or unmapped characters.
