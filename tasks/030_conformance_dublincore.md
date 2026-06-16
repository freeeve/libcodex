# 030 — Conformance: Dublin Core

Verify the oai_dc XML validates against the OAI schema and that elements and type
values follow DCMI.

## References
- OAI `oai_dc` schema: http://www.openarchives.org/OAI/2.0/oai_dc.xsd
- DCMI Metadata Terms (the 15 elements): https://www.dublincore.org/specifications/dublin-core/dcmi-terms/
- DCMI Type Vocabulary: https://www.dublincore.org/specifications/dublin-core/dcmi-type-vocabulary/

## Checks
- Namespaces `http://www.openarchives.org/OAI/2.0/oai_dc/` and
  `http://purl.org/dc/elements/1.1/`; the `oai_dc:dc` wrapper.
- Only the 15 DCMES elements are emitted; element repetition allowed; no ordering
  requirement but a stable canonical order.
- `dc:type` values come from the DCMI Type Vocabulary; `dc:language` codes.
- XML 1.0 character validity (already enforced — confirm); the DC-JSON form uses
  the same element names.

## Verification
- Validate `Encode` output against `oai_dc.xsd` with `xmllint --schema`.
- Check `dc:type` outputs against the DCMI Type term list.

## Acceptance
- Generated oai_dc validates against the OAI schema; type/language values cited
  against DCMI.

## Depends on
- dublincore (task 018).
