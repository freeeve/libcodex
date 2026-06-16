# 023 — Multilingual and accessibility support (done)

## Goal
Add the multilingual and accessibility features a bibliographic library is
expected to expose, building on the full MARC-8 script support from task 022.

## Delivered (four pieces, requested together)
1. **008 typed accessors** (`fixed_fields.go`): `Record.Control008()` →
   `Control008` with `DateEntered`, `DateType`, `Date1`, `Date2`, `Place`,
   `Language`, `CatalogingSource`, and the material-aware `FormOfItem`
   (`IsLargePrint` / `IsBraille`). Out-of-range positions degrade to empty/zero.
2. **880 `$6` vernacular linkage** (`linkage.go`): `Field.Link()` parses the
   linkage (tag, occurrence, script code, right-to-left orientation);
   `Record.AlternateGraphic(field)` resolves the linked partner in either
   direction; `Record.Vernacular(tag, code)` reads the original-script value.
   `Linkage.ScriptName()` names the `$6` script code (the MARC-8 set designations).
3. **Accessibility accessors** (`accessibility.go`): `Record.Accessibility()`
   gathers the 008 form of item, 007 tactile category, and the 341 Accessibility
   Content and 532 Accessibility Note fields into an `Accessibility` value.
4. **schema.org export** (`schemaorg` package): MARC → schema.org JSON-LD
   (`Book`/`CreativeWork` by leader type) carrying the common bibliographic fields
   plus accessibility metadata mapped to `accessMode`, `accessibilityFeature`
   (largePrint, brailleViaTouch) and `accessibilitySummary`. Hand-rolled JSON,
   stdlib only; `Writer` implements `codex.RecordWriter`. Language codes mapped to
   BCP-47 where common.

## Tests
- Core (`codex`) accessors back to 100% coverage.
- `schemaorg` ~95%: well-formedness (parsed back with `encoding/json`), crosswalk,
  accessibility mapping, writer error paths, golden file, benchmark, and
  `FuzzFromMARC` (clean over 6M executions).
- `schemaorg` added to the cross-package `TestExportConvertersCanonical` smoke
  test over the real MARC-8 corpus.

## Notes
- The `$6` script codes reuse the MARC-8 set designations from task 022, so script
  naming is shared knowledge.
- README documents the new accessors and the schema.org accessibility mapping.
