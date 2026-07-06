# 090 -- Classification display label alongside the code

Filed from libcatalog (tasks/142, 2026-07-06). bibframe.Classification
carries {Class, Value, ItemPortion, Edition, Source} -- no display label --
so a deployment hydrating a coded scheme (e.g. BISAC) must choose between
putting the code in Value (right for MARC 084) and the human text (right
for a catalog facet). queerbooks currently ships the text in Value and
documents the MARC trade-off.

## Change

- Add `Label string` to `bibframe.Classification` (bibframe/bibframe.go:198)
  -- the human display text for the coded Value, optional.
- Graph emission: hang the label on the Classification node as `rdfs:label`
  (the display-only channel). libcatalog's projector already reads it there
  (libcatalog project/project.go, schema v9): facets show the label, exports
  and the taxonomy keep the code.
- Reader (graph -> Classification): populate Label from the node's
  rdfs:label.
- MARC crosswalk: 084/072/082/050 $a stays the code; the label does NOT get
  a MARC subfield on the way out (no standard channel), so a
  Classification round-tripped through MARC loses Label -- document that.
  Optionally: $2-source-aware rendering could regenerate labels for known
  schemes on read, but that's a separate concern.

## Downstream (for context, not this repo)

- libcatalog's OverDrive ingest has `BISAC.Description` from the feed and
  will set `Label: b.Description` once this field exists
  (ingest/overdrive/bibframe.go).
- queerbooks then flips its BISAC hydration back to codes in Value with the
  heading in Label.

## Done -- implementation notes (2026-07-06)

- `bibframe.Classification.Label` added (bibframe/bibframe.go); emitted as
  `rdfs:label` on the node (bibframe/shape.go emitClassification), kept
  separate from the coded `bf:classificationPortion`.
- Graph -> MARC crosswalk unchanged and now documents that Label is dropped
  (reader_crosswalk.go classificationFields). Tests in
  classification_fidelity_test.go: label emitted, omitted when empty, and lost
  through MARC.

### Why the slot lives in libcodex (sourcing vs representation)

Evidence: a real OverDrive MARC export (ME-15711 Queer Liberation Library
eBook, 67 records) carries BISAC as codes only --
`084 ## $aYAF010010 $aYAF010140 $aYAF010170 $2bisacsh` -- with zero heading
text. Human headings appear only as `650 _7 $2 OverDrive`, a separate,
coarser vocabulary (33 broad terms; 0 records use `650 $2 bisacsh`). The
per-code BISAC heading ("Young Adult Fiction / ...") is not in the MARC at
all -- it lives in the OverDrive feed's `BISAC.Description`.

So MARC cannot round-trip "BISAC = code + heading" either way: 084/072 holds
the code and drops the heading; 650 holds a heading and drops the code (and
mis-types a shelving classification as a topical subject). libcodex does not
*source* the label (no BISAC table in the repo -- correct); it only provides
a *representation* slot, which is in scope because bibframe exposes a public
BIBFRAME model + Graph() for non-MARC producers and `bf:Classification` +
`rdfs:label` is standard vocabulary. Label being empty from MARC input is
correct, not a smell. Decided to keep this design as-is; a derive-from-code
scheme table (regenerate headings from code+$2) is a possible later task only
if a MARC-only path ever needs headings.
