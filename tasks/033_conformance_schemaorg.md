# 033 — Conformance: schema.org JSON-LD

Verify the JSON-LD is valid, uses defined schema.org terms, and that the
accessibility property values come from the controlled vocabulary.

## References
- schema.org vocabulary (types and properties): https://schema.org/
- JSON-LD 1.1 syntax.
- schema.org accessibility metadata (controlled values for `accessibilityFeature`,
  `accessMode`, `accessibilityHazard`): the W3C/DAISY a11y metadata guidance and
  https://schema.org/accessibilityFeature

## Checks
- `@context` is `https://schema.org`; `@type` is a defined type
  (`Book`, `Map`, `Movie`, `AudioObject`, `MusicRecording`, `ImageObject`,
  `SoftwareApplication`, `CreativeWork`, …).
- Properties used (`name`, `author`, `publisher`, `datePublished`, `isbn`,
  `inLanguage`, `about`, `genre`, `bookEdition`, `url`, `description`,
  `accessMode`, `accessibilityFeature`, `accessibilitySummary`) are valid for the
  type.
- `accessibilityFeature` values are exact controlled terms (e.g. `largePrint`,
  `brailleViaTouch`, `captions`, `audioDescription`); `accessMode` values are
  `textual`/`visual`/`auditory`/`tactile`; `inLanguage` uses BCP-47 where mapped.

## Verification
- Validate JSON-LD parses/expands with a JSON-LD processor.
- Run output through the schema.org validator / Google Rich Results test.
- Check the accessibility term spellings against the controlled vocabulary list.

## Acceptance
- JSON-LD validates and expands; types/properties and a11y values cited against
  schema.org and the a11y vocabulary.

## Depends on
- schemaorg (task 023).
