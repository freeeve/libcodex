# 025 — Conformance: MARCXML

Verify MARCXML output validates against the official LoC schema and that decoding
accepts every schema-valid document.

## References
- LoC MARCXML schema (slim): https://www.loc.gov/standards/marcxml/schema/MARC21slim.xsd
- MARCXML design + documentation: https://www.loc.gov/standards/marcxml/

## Checks
- Namespace `http://www.loc.gov/MARC21/slim` on `collection`/`record`.
- Element order within a record: `leader`, then `controlfield*`, then `datafield*`;
  `datafield` contains `subfield*`.
- Attribute constraints: `tag` is 3 characters; `ind1`/`ind2` are single
  characters (blank defaults); `subfield@code` is one character; the schema's
  `idType`/`indicatorDataType`/`subfieldcodeDataType` patterns.
- Only XML 1.0 legal characters are emitted; invalid control characters are
  rejected on encode (already enforced — confirm against the schema's facets).
- Decoder accepts schema-valid documents including unusual but legal element/
  attribute orderings and ignorable whitespace.

## Verification
- Validate `Encode` output against `MARC21slim.xsd` with `xmllint --schema`
  (or an equivalent stdlib-driven check in a test guarded behind a build tag).
- Round-trip the LoC published MARCXML example records.

## Acceptance
- A test (or documented procedure) showing generated MARCXML validates against the
  LoC schema, plus round-trip of the LoC examples.

## Depends on
- marcxml (task 006).
