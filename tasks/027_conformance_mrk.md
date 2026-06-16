# 027 — Conformance: MARCMaker (.mrk)

Verify the mnemonic text format matches the LoC MARCMaker/MARCBreaker conventions
and interoperates with MarcEdit's `.mrk`.

## References
- LoC MARCMaker/MARCBreaker (makrbrkr): https://www.loc.gov/marc/makrbrkr.html
- MarcEdit `.mrk` conventions (the widely used de-facto implementation).

## Checks
- `=LDR  ` leader line and `=TAG  ` field lines (two spaces after the tag).
- Blank indicators rendered as `\`; subfield delimiter `$` followed by the code.
- Mnemonic escapes for literal delimiter/brace characters
  (`{dollar}`, `{lcub}`, `{rcub}`) and numeric character references
  (`&#xHHHH;` / `&#DDDD;`) on decode.
- One blank line between records; control fields have no indicators/subfields.
- Byte-transparency of UTF-8 data; line breaks within a datum are rejected on
  encode (already enforced — confirm).

## Verification
- Round-trip `.mrk` produced by MarcEdit for the same records.
- Cross-check a few records' `.mrk` rendering against MarcEdit output.

## Acceptance
- Documented conformance with MarcEdit-interop fixtures and round-trip tests.

## Depends on
- mrk (task 008).
