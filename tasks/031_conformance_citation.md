# 031 — Conformance: RIS and BibTeX

Verify the citation output parses in the reference managers and tools that consume
each format.

## References
- RIS format specification (tag list, reference types, `ER` terminator) as used by
  Zotero/EndNote/Mendeley.
- BibTeX format: entry types and standard fields (the `btxdoc` / `tame the BeaST`
  references); LaTeX special-character escaping rules.

## Checks
- RIS: line format `XX␠␠-␠value`; `TY` is the first tag with a value from the
  reference-type list; `ER  - ` terminates each record; tags used (TI, AU, PY, DA,
  PB, CY, SN, KW, LA, AB, UR) are valid.
- BibTeX: `@type{key, field = {value}, ...}`; entry types and field names are
  standard; the cite key is stable and ASCII; special characters
  (`{ } & % $ # _ \ ~ ^`) escaped so the entry parses.

## Verification
- Parse the generated `.ris` with a RIS parser (or import into Zotero manually for
  a spot-check) and the generated `.bib` with `bibtex`/`biber` (no errors).
- Confirm round-trip of key fields through a parser back to the same values.

## Acceptance
- Generated RIS and BibTeX parse cleanly in a reference parser; tag/type/field
  choices cited against the specs.

## Depends on
- citation (task 019).
