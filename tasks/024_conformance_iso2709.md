# 024 — Conformance: ISO 2709 / MARC 21 record structure

Thorough re-verification of the binary codec against the standard (extends the
initial pass in task 003).

## References
- ISO 2709:2008 (information and documentation — format for information exchange).
- LoC "MARC 21 Specifications for Record Structure, Character Sets, and Exchange
  Media": https://www.loc.gov/marc/specifications/specrecstruc.html

## Checks
- Leader: all 24 positions; record length [0:5] and base address [12:17] computed
  correctly; indicator count (leader/10) and subfield code count (leader/11) are
  honored rather than assumed; the entry map (leader/20-23 = "4500") — confirm the
  directory entry layout (4-digit length, 5-digit start, 0 part) is read from the
  map, not hardcoded, or document the fixed-map assumption explicitly.
- Directory: 12-byte entries; entries whose data range falls outside the field
  area are skipped without failing the record; out-of-order entries.
- Field/record terminators (0x1E / 0x1D); records with and without the trailing
  record terminator; trailing bytes after the terminator.
- Control vs data field boundary at tag < "010"; data fields with 0, 1 or 2
  indicators; repeated subfield codes; empty subfields; a subfield delimiter with
  no following code.
- Limits: a field over 9999 bytes and a record over 99999 are rejected on encode;
  a directory length field that overflows.
- Byte-exact round trip on already-normalized input; idempotent re-encode.

## Verification
- Run against real LoC MARC 21 sample records and the existing pymarc corpus.
- Cross-check field offsets/lengths a second tool (e.g. `yaz-marcdump`, pymarc)
  reports for the same files.
- Extend the fuzz corpus with structurally adversarial directories.

## Acceptance
- Documented conformance notes (what is enforced, what is intentionally lenient),
  with tests for each structural rule above.

## Depends on
- iso2709 (tasks 002, 004).
