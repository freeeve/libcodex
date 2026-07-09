# 102 -- 490$v: encode/decode bf:seriesEnumeration alongside seriesStatement

Filed from libcat on 2026-07-09 (cross-repo ask).

libcat v0.72.0 (tasks/221/222) lets catalogers record a series
enumeration -- the 490$v volume/part -- as `bf:seriesEnumeration` on the
Instance, next to the `bf:seriesStatement` you already round-trip as
490. Today the crosswalk carries only $a:

- encode: `seriesStatement(f)` (bibframe.go:1403) renders a 490 into one
  seriesStatement literal; $v is not read.
- decode: `reader_crosswalk.go:62` emits `seriesField(stmt)` with $a
  only, so an editorial enumeration never reaches exported MARC.

The ask, both directions:

- **encode** (MARC -> BF): 490$v -> `bf:seriesEnumeration` literal on
  the Instance ($a keeps its current shape; repeat $v handling is your
  call -- first-wins matches how libcat's profile bounds the field to
  max 1).
- **decode** (BF -> MARC): when the Instance carries seriesEnumeration,
  append `$v` to the 490 the seriesStatement already produces; a bare
  enumeration with no statement can stay unemitted.

Predicate IRI: `http://id.loc.gov/ontologies/bibframe/seriesEnumeration`
(BF 2.0 core). No urgency -- our fidelity table does not gate 490$v, and
the editor field works end-to-end in the grain, catalog.json, and the
OPAC without it. It only matters for MARC export parity.