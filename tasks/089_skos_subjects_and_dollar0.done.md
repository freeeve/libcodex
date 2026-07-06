# 089 -- crosswalk: read SKOS-shaped subjects natively; carry $0 both ways

Filed from libcatalog (its tasks/136, 2026-07-06). libcatalog-side shim
landed there (bibframe/marcsubjects.go) -- this task makes it unnecessary.

## Reader side (BIBFRAME -> MARC, reader_crosswalk.go subjectFields)

- A `bf:subject` object with `skos:prefLabel` but no `rdfs:label` currently
  produces nothing. Read the SKOS shape natively: prefLabel (English first)
  as the heading, default type Topic (650) when no rdf:type is present.
- Emit `$0 <authority-iri>` on 6xx headings whose subject node is an IRI --
  subjectFields never writes $0 today, and the authority link is the part
  ILS consumers actually keep.
- Thesaurus for ind2/$2: keep honoring `bf:source`; consider deriving it
  from well-known IRI prefixes (id.loc.gov/authorities/subjects, homosaurus,
  id.worldcat.org/fast) when no source node exists.
- `skos:broader` maps to nothing (subdivisions are a different axis --
  libcatalog docs/marc-fidelity.md documents the choice).

## Writer side (MARC -> BIBFRAME, FromRecord)

- A 6xx carrying `$0 <uri>` should mint `bf:subject <uri>` (IRI object, not
  a blank Topic node) plus `rdfs:label`/`skos:prefLabel` from the heading,
  so ingesting exported MARC keeps the authority link instead of degrading
  it to a labeled blank node.

## Reference

libcatalog bibframe/marcsubjects.go holds the shim (prefix->code table,
label pick, $0 injection) and marcsubjects_test.go the expected output:
`650 _7 $a Label $2 homosaurus|fast|lcsh $0 <iri>`.

## Done (2026-07-06)

Reader (reader_crosswalk.go subjectFields):
- Reads `skos:prefLabel` when `rdfs:label` is absent (new `pPrefLabel`,
  `skosNS` in bibframe.go/reader.go).
- Untyped subject node defaults to Topic (650).
- Emits `$0` when the subject node is an IRI (new shared
  `appendThesaurusAndAuthority`; `headingField`/`nameHeadingField` take an
  authority arg).
- Derives the thesaurus from well-known IRI prefixes via `sourceFromIRI`
  (lcsh / lcshac / fast / homosaurus / mesh) when no `bf:source` node exists.

Writer (FromRecord):
- `Subject.Authority` added; `subjectAuthority` reads a URI-shaped 6xx `$0`
  (ignores record-control `$0` like `(DLC)...`).
- `emitSubject` mints an IRI subject node (`subjectIRIVal`) when Authority is
  set, so ingesting exported MARC keeps the authority link instead of
  degrading to a labeled blank node.

Tests: subject_authority_test.go -- $0 read (URI vs control number),
Encode->Decode $0/$2 round-trip, native SKOS (prefLabel-only IRI ->
650 _0 $a $0), and the sourceFromIRI prefix table. `skos:broader` remains
unmapped (subdivisions are a separate axis), as noted above.
