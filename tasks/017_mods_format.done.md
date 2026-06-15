# 017 — MODS format (mapping layer)

## Goal
Read and write MODS (Metadata Object Description Schema), the Library of Congress
XML standard that is richer than MARCXML and near-lossless from MARC.

## Why this is different from the four MARC codecs
`iso2709`/`marcxml`/`marcjson`/`mrk` are serializations of the *same* MARC model,
so they implement `RecordReader`/`RecordWriter` directly. MODS is a *different*
data model (titleInfo, name, originInfo, subject, …), so it needs a **mapping
layer** (a crosswalk), not just a codec. The LoC publishes the MARC↔MODS
crosswalk to follow.

## Scope
- `mods` package using `encoding/xml`.
- A MARC→MODS mapping for the common fields (leader/008 → typeOfResource/genre;
  1xx/7xx → name; 245 → titleInfo; 260/264 → originInfo; 3xx → physicalDescription;
  5xx → note; 6xx → subject; 020/022 → identifier).
- Decide direction: MARC→MODS first (export); MODS→MARC is harder and lossy.
- Because the mapping is lossy/opinionated, document exactly what is and isn't
  carried; this does NOT implement `RecordReader`/`RecordWriter` over the bare
  MARC model — it is a converter `MODS(*codex.Record) (..., error)`.

## Status — done
- `mods` package: `FromRecord(*codex.Record) *MODS`, `Encode`, a `Writer`
  (implements `codex.RecordWriter`, wraps `<modsCollection>`, needs `Close`) so it
  plugs into `codex.Convert`, and `WriteFile`. `encoding/xml`; namespaced output.
- Crosswalk (common fields): 245/130/240 → titleInfo; 1xx/7xx → name (with role
  and date); leader/06 → typeOfResource; 260/264/250/008 → originInfo;
  300 → physicalDescription; 008/041 → language; 5xx → note; 6xx → subject (topic/
  geographic/temporal/genre subdivisions, name subjects, authority from ind2);
  020/022/024/856 → identifier; 001 → recordInfo. ISBD trailing punctuation
  trimmed. Out-of-crosswalk fields are not carried (documented).
- Tests: structural assertions + `xml.Unmarshal` round-check, namespace, Writer
  collection, `codex.Convert(iso2709 → mods)`, empty record, file I/O, golden
  (`testdata/sample.mods.xml`), `FuzzFromMARC`. Coverage 83.3%.

## Performance (done)
| Benchmark | allocs | ns/op | note |
|-----------|--------|-------|------|
| Encode    | 61 → **51** | 8684 → 7308 | single-pass `FromRecord` |
| WriterStream (100) | 5903 → 4903 | — | ~49/record |

- `FromRecord` rewritten to a single pass over the fields (was calling
  `DataFields(tag)` ~20 times, each a full scan + slice alloc). The remaining cost
  is the `encoding/xml` struct-marshal reflection floor (~46% of allocs), the same
  trade-off as marcxml decode; not worth hand-rolling the deeply-nested MODS XML
  for an export-only converter.

## Fuzz (done)
`FuzzFromMARC`: any decodable MARC record converts to **well-formed XML** without
panicking. Campaign clean (7.4M execs).

## Acceptance — met
- MARC→MODS crosswalk documented and implemented for the common fields; output is
  valid, namespaced MODS; golden-file test included.

## Depends on
- 002 (after 016)
