# 081 -- Round-trip: reconstruct the remaining lost MARC fields on Decode

## Context (filed from libcatalog)

libcatalog's measured loss gate (`bibframe/roundtrip_test.go` +
`docs/marc-fidelity.md`, its tasks/003 and tasks/053) tracks which MARC tags
survive `Encode -> Decode` against the vendored OverDrive MARC Express
samples. The v0.9.0 crosswalk gains moved **008, 336, and 500** from lost to
kept -- this task is the rest of the reconstructable set. libcatalog also has
a stale-table guard now (`TestMARCRoundTripLossTableCurrent`): each fix here
will surface there as a "known-lost field now survives" failure, prompting
its table/doc update.

Downstream stake: libcatalog's cataloging editor preserves crosswalk-dropped
fields verbatim as opaque `lcat:marcVerbatim` literals (its tasks/049), so
nothing is blocked -- every tag reconstructed here shrinks that opaque
sidecar and becomes graph-native, editable data instead.

## Scope (in likely value order)

1. **511 / 521 / 533 / 538 specialized notes.** Content already survives as
   bf:Note (the 5XX -> bf:Note work); the decode side emits a generic note
   tag. Carry a typed noteType (bflc or rdfs:label convention) on Encode and
   map it back to the original tag on Decode.
2. **490 series statement.** BIBFRAME models series (bf:hasSeries /
   seriesStatement); Encode appears to keep it graph-side already or can --
   Decode should re-emit 490 (and 8XX where applicable).
3. **776 additional physical form.** v0.9.0 built 76X-78X -> bf:relation on
   Encode; add the Decode-side reconstruction to 776 (and siblings if cheap).
4. **006 / 007 coded elements.** Same shape as the 008 reconstruction that
   shipped in v0.9.0: typed properties -> packed positions, driven by the
   material-type tables. 306 (playing time) and 347 (digital file
   characteristics) can ride along as carrier-detail properties.

## Explicit non-goals (do not file follow-ups for these)

- **040 cataloging source** -- consumers model provenance out-of-band
  (libcatalog: named graphs); reconstructing 040 would fabricate provenance.
- **037 / 084 vendor conventions** (OverDrive Reserve ID, BISAC-in-084) --
  the useful direction is *reading* them in FromRecord (tasks/057), not
  round-tripping; direct-JSON ingest paths keep them natively.

## Acceptance

- Each reconstructed tag passes a round-trip test on the MARC Express
  samples here, and libcatalog's `TestMARCRoundTripLossTableCurrent` flags
  the stale entry (its table then moves the tag to coreFields).

## Result

Scope items 1-3 landed in full; item 4 landed 306/347 and split the 006/007
packed reconstruction to task 082.

- **Specialized notes**: 511 -> `performers` (Work), 521 -> `audience` (Work),
  533 -> `reproduction` (Instance), 538 -> `systemDetails` (Instance), all via
  the 072 `bf:noteType` convention (`noteTypeForTag`/`tagForNoteType`), so each
  decodes back to its original tag. Note labels now join every subfield
  (`noteLabel`) -- a multi-subfield 533 keeps its place/agency details.
- **490**: `Instance.SeriesStatements` -> `bf:seriesStatement` (litList).
  `$v` rejoins after the ISBD " ; " separator and `splitSeriesStatement`
  restores `$a`/`$v` on decode; the join/split is idempotent, so the
  graph-level round-trip is stable under fuzzing.
- **776 `$z`**: `Relation.ISBN` -> `bf:Isbn` on the associated resource; the
  OverDrive print/ebook pairing (776 with only `$c`/`$z`) now survives instead
  of being skipped. Reverse routes identifier class -> `$x` (Issn) / `$z`
  (Isbn).
- **306/347**: `Instance.Duration` -> `bf:duration`; 347 `$a`/`$b` ->
  `bf:digitalCharacteristic` -> `bflc:FileType`/`bflc:EncodingFormat` labeled
  nodes, interleaving preserved. (`$2 rda` is not round-tripped -- emitting it
  unconditionally would fabricate the source.)
- **Bug found by the new gate**: repeated `bf:relatedTo` and `bf:relation`
  children collided on one JSON-LD object key (third instance of the
  duplicate-key class); both now emit as lists.

### Loss-gate regression matrix (`bibframe/lossgate_test.go`)

Mirrors libcatalog's fidelity gate upstream: a fully populated kitchen-sink
record and the 11 `testdata/realdata` LC records round-trip through **all four
BIBFRAME serializations** (RDF/XML, JSON-LD, Turtle, N-Triples), asserting
every tag is kept (`coreTags`, 48 tags), transformed (`037->024`, `084->072`;
provision-typed `264->260`), or lost (`003/005/040/310` -- the stale guard
that fails when future work makes one survive). The kitchen sink also runs
through the lossless codecs (iso2709/marcxml/marcjson/mrk) with exact-equality
assertions.

Deliberately not reconstructed (task non-goals): 040 (would fabricate
provenance), 037/084 as vendor conventions (read into the graph, transformed on
decode). 006/007 packed-position reconstruction -> task 082.
