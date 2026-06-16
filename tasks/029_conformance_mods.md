# 029 — Conformance: MODS

Verify MODS output validates against the LoC MODS schema and that the crosswalk
follows the official MARC-to-MODS mapping.

## References
- MODS schema (current 3.x): https://www.loc.gov/standards/mods/mods-schemas.html
- LoC MARC 21 to MODS mapping: https://www.loc.gov/standards/mods/mods-mapping.html

## Checks
- Namespace `http://www.loc.gov/mods/v3` and `version` attribute.
- Element order and nesting per the schema (`titleInfo`, `name`, `originInfo`,
  `subject`, `identifier`, `recordInfo`, …); attribute enumerations
  (e.g. `name@type`, `subject@authority`, `identifier@type`).
- The crosswalk dispatch (which MARC field maps to which MODS element) matches the
  LoC mapping for the covered fields; documented gaps for uncovered fields.

## Verification
- Validate `Encode` output against the MODS XSD with `xmllint --schema`.
- Spot-check a few records against the LoC mapping examples.

## Acceptance
- Generated MODS validates against the schema; crosswalk decisions cited against
  the LoC mapping with documented coverage.

## Depends on
- mods (task 017).
