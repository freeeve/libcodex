# 028 — Conformance: MARC-8 character sets

Verify every MARC-8 code point and escape behavior against the authoritative LoC
tables (extends task 022, which brought all sets into scope).

## References
- LoC MARC-8 / "Character Sets and Encoding Options":
  https://www.loc.gov/marc/specifications/speccharintro.html
- LoC code tables (already the generation source):
  https://www.loc.gov/marc/specifications/codetables.xml

## Checks
- The generated tables are a faithful reproduction of `codetables.xml` (regenerate
  and diff; pin a checksum of the source or the generated file).
- The hand-maintained ANSEL table matches the LoC Extended Latin table exactly,
  including the alif/ayn remappings and the euro/eszett additions.
- Escape designations: G0/G1 single-byte sets and the multibyte EACC designation;
  ASCII/ANSEL reinstatement; non-standard designations handled without crash.
- Combining order (mark before base) for every set; NFC composition for Latin;
  decomposed output documented for non-Latin.
- EACC completeness (~15,700 entries) and the unmapped-triple/U+FFFD path.

## Verification
- Decode the full pymarc multiscript corpus and assert no regressions
  (already partly covered); where a reference UTF-8 is available, compare exactly.
- Property: decode(encode(x)) stability across all sets (fuzz, already clean).

## Acceptance
- Documented faithfulness to the LoC tables (generation diff + ANSEL audit), with
  the spec citations recorded.

## Depends on
- internal/marc8 (tasks 016, 022).
