# 039 -- BIBFRAME: carrier/format on Instance

## Motivation

`bibframe.Instance` models titles, edition, provision, extent, identifiers, and
electronic locator, but not the **carrier/format** -- ebook vs audiobook vs print.
In BIBFRAME that distinction is Instance-level (`bf:media` / `bf:carrier`), not
Work-level.

A consumer (libcatalog) clusters multiple editions into one Work; it needs each
Instance to state its own format so a "format facet" is correct. Today it can only
set a Work content class, which is lost when editions cluster (libcatalog
`tasks/011`).

## Change

Add a carrier/format to `Instance`, rendered on the Instance node in every
serialization (graph/N-Quads, RDF/XML, JSON-LD). Suggested minimal shape:

- `Instance.Carrier string` (e.g. RDA carrier "online resource", or a term like
  "audiobook"/"ebook"), emitted as `bf:carrier [a bf:Carrier; rdfs:label <...>]`,
  and/or `Instance.Media` -> `bf:media`. Follow the existing labeled-node pattern
  (`graphBuilder.labeled`), like Extent/Place.
- `FromRecord` may populate it from 337/338 where present; the struct field is the
  goal so a direct (non-MARC) caller can set it.

## Acceptance

- [x] `Instance.Carrier` and `Instance.Media` fields added; emitted (as
      `bf:carrier [a bf:Carrier; rdfs:label …]` / `bf:media [a bf:Media; …]`) when
      set, omitted when empty; all four serializers stay isomorphic.
- [x] Round-trip coverage for an Instance with a carrier/media
      (`TestInstanceCarrierMedia`): 337/338 -> struct -> all serializations ->
      Decode -> 337/338.

Consumer: libcatalog `tasks/011`.

## Resolution

- `Instance` gains `Media` and `Carrier` string fields (RDA media/carrier type).
- `FromRecord` populates them from 337 $a and 338 $a; a direct (non-MARC) caller
  can set them on the struct.
- All three emitters render them on the Instance node via the labeled-node
  pattern (graph builder `labeled`, `labeledXML`, `appendLabeledJSON`), between
  extent and identifiers, so the N-Triples/Turtle, RDF/XML and JSON-LD stay
  isomorphic.
- The reverse crosswalk maps `bf:media` -> 337 and `bf:carrier` -> 338, so the
  shape round-trips.
- New predicates `pMedia`/`pCarrier`. Single-valued strings (the task's minimal
  shape); the golden sample is unchanged since it carries no 337/338.
