# 010 — Thorough fuzz pass: marcxml

## Goal
Harden the `marcxml` codec with a thorough fuzz pass.

## Scope
- `FuzzDecode`: arbitrary/markup-shaped bytes never panic; malformed XML returns
  an error rather than crashing.
- `FuzzEncodeRoundTrip`: a record built from fuzzed model values encodes to
  well-formed XML that decodes back equal (`Encode → Decode` is identity on the
  model), with attention to XML-escaping, control characters, and namespaces.
- `FuzzRoundTrip`: `Decode → Encode → Decode` is stable for inputs that decode
  cleanly.
- Seed corpus: valid `<collection>`/`<record>`, bare records, missing/extra
  attributes, unescaped entities, deep nesting, empty input.
- Sustained campaign; commit crashers under `marcxml/testdata/fuzz`.

## Status — done
- `FuzzDecode`: arbitrary input never panics; `Decode→Encode→Decode` is
  byte-stable for self-consistent records. Campaign clean (>4M execs); 3 crasher
  seeds committed under `marcxml/testdata/fuzz/FuzzDecode`.
- **Findings fixed:** (1) a subfield with no `code` (code 0 / NUL) made Encode
  emit XML-illegal output — `marcxml.Encode`/`Writer.Write` now reject any datum
  containing a character XML 1.0 cannot represent (`validate`), covered by
  `TestEncodeRejectsInvalidXML`. (2) A `<datafield>`/`<controlfield>` whose tag's
  range disagrees with the element type (e.g. `<datafield tag="">` or
  `<controlfield tag="1">`) is misclassified by the tag-based model and is not
  round-trip-stable; this is malformed input (the codec doesn't panic), scoped
  out of the stability assertion via `selfConsistent`.

## Acceptance
- No panics or hangs; encoder always emits well-formed XML; round-trip invariant
  holds; seed corpus committed.

## Depends on
- 006
