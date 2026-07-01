# 037 -- Add a source/scheme to BIBFRAME Identifier and Classification

## Motivation

`bibframe.Identifier` and `bibframe.Classification` model only `{Class, Value}`.
That is enough for ISBN/ISSN and LCC/DDC, where the class implies the scheme, but
not for source-qualified vocabularies. A downstream consumer (libcatalog's direct
OverDrive provider) needs to express:

- BISAC classifications: `bf:Classification` + `bf:classificationPortion` **plus
  `bf:source` "bisacsh"**.
- Provider-local identifiers with a scheme (e.g. an OverDrive Reserve ID) that
  must be distinguishable from a bare `bf:Identifier`.

Both are standard BIBFRAME: a classification or identifier node may carry
`bf:source [a bf:Source; rdfs:label "…"]`.

## Change

Add an optional `Source string` to `Identifier` and `Classification`. When
non-empty, emit a `bf:source` node (`a bf:Source; rdfs:label <source>`) on the
identifier/classification node, in every serialization:

- `graph.go` (N-Triples/N-Quads/Turtle basis)
- `jsonld.go`
- `rdfxml.go`

`FromRecord` may set `Source` where MARC carries it (e.g. `072 $2`, `020`/`024`
`$2`, classification `$2`), but that is optional; the struct field is the goal.

## Acceptance

- [ ] `Identifier.Source` and `Classification.Source` fields added.
- [ ] `bf:source` emitted when set, omitted when empty (no empty nodes).
- [ ] All serializers updated; `TestEncodersIsomorphic` stays green.
- [ ] Round-trip/golden tests cover a sourced identifier and classification.

Consumer: libcatalog `tasks/008`.
