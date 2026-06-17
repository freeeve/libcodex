# 034 — UNIMARC support (crosswalk + accessors + ISO 5426)

Captured decision: add UNIMARC at the "crosswalk + accessors + ISO 5426 legacy
encoding" scope (the chosen option).

## Background
UNIMARC (IFLA) shares the ISO 2709 physical structure with MARC 21, so `iso2709`
already reads/writes the raw bytes. What differs is the data dictionary (different
tag semantics) and the character encoding (legacy UNIMARC uses ISO 5426 / ISO 5428
for Cyrillic, not MARC-8/ANSEL; modern UNIMARC is UTF-8, declared in field 100/26-29
rather than leader byte 9).

## Scope
- A `unimarc` package with:
  - **Accessors** over a `codex.Record` for the common UNIMARC fields
    (200 title, 700/701/710 responsibility, 010 ISBN, 011 ISSN, 101 language,
    102 country, 205 edition, 210/214 publication, 215 description, 6xx subjects,
    330 summary, 856 locator) and the 100/101 coded-data block.
  - **`ToMARC21(*codex.Record) *codex.Record`** — re-tag a UNIMARC record to MARC 21
    equivalents (200→245, 010→020, 011→022, 205→250, 210/214→260/264, 215→300,
    101→008/041, 606→650, 607→651, 600→600, 601→610, 608→655, 610→653, 330→520,
    700/701→100/700, 710→110/710, 856→856) so a UNIMARC record flows into every
    existing exporter (mods, dublincore, citation, bibframe, schemaorg).
- **ISO 5426 transcoding** as a new internal codec (`internal/iso5426`), the
  UNIMARC analog of `internal/marc8`: decode ISO 5426 (and ISO 5428 Cyrillic) to
  UTF-8 and back, combining marks before base as in ISO 5426. A UNIMARC reader
  selects the charset from field 100/26-29 (falling back to UTF-8 when leader/9
  indicates Unicode).

## Out of scope (for now)
- The reverse MARC 21 → UNIMARC crosswalk (a separate option, not chosen).
- The full UNIMARC data dictionary beyond the common fields.

## Acceptance
- `unimarc` package converting real UNIMARC records into MARC 21 fields that the
  existing exporters accept; UNIMARC accessors; ISO 5426/5428 round-trip with the
  same optimize/profile/fuzz rigor as `marc8`. A conformance check against the
  IFLA UNIMARC Manual field list and the ISO 5426 code table.

## Depends on
- iso2709 (structure), and the export converters it feeds.
