# 092 -- SKOS concept schemes: view + MARC authority crosswalk

Filed 2026-07-06 from the "print the homosaurus vocab" thread. libcodex reads
SKOS subjects inside BIBFRAME (task 089) but cannot handle a standalone SKOS
concept scheme (e.g. a homosaurus .nt dump). Add first-class support.

## Design (agreed ergonomics)

New `skos` library package + a dedicated `libcodex skos` subcommand.

```
libcodex skos [-i fmt] [-o fmt] [-n N] [file...]
  default (-o omitted): a readable concept view with a summary header
  -o marc|marcxml|marcjson|mrk: crosswalk each concept to a MARC authority record
```

### Library `skos`

- `Parse([]byte) ([]Concept, error)` / `Read(io.Reader)` -- parse RDF
  (nt/turtle/rdfxml/jsonld, autodetected via the rdf package), collect
  skos:Concept nodes, resolve broader/narrower/related to target labels.
- `Concept{URI, ID, Scheme, Pref, Alt []Label, Broader/Narrower/Related []Ref,
  Notes []Label}`; `Label{Text, Lang}`, `Ref{URI, ID, Label}`.
- `Concept.PrefLabel()` -- English-preferred heading.
- `Concept.Record() *codex.Record` -- MARC authority crosswalk:
  - Leader/06 `z`; 001 = ID; 024 7 $a <uri> $2 uri
  - 150 $a English prefLabel
  - 450 $a altLabels + non-English prefLabels (see-from)
  - 550 $w g/h $a broader/narrower (label-resolved) $0 <uri>; 550 $a related
  - 680 $i scopeNote / rdfs:comment

### CLI view

Summary header surfaces version/scheme drift (the reason this thread started --
the "v5" homosaurus file is all v4 IRIs with a v3/v4 inScheme split):

```
# N concepts
# IRI base:   https://homosaurus.org/v4/ (N)
# inScheme:   https://homosaurus.org/v3 (3744), https://homosaurus.org/v4 (416)  [mixed]
# label langs: en (N), es (N), ...

homoit0000001  5-alpha reductase deficiency  [en]
  uri: https://homosaurus.org/v4/homoit0000001
  alt: 5-ARD [en], 5-ARD [es]
  broader: Intersex (homoit0000669)
```

## Status

Done. `skos` package (skos.go/parse.go/authority.go) + `libcodex skos`
subcommand (cmd/libcodex/skos.go), tests in skos/skos_test.go and the CLI test.
Verified on ~/Downloads/v5.nt: 4160 concepts view with the [mixed] inScheme
header surfacing the v3/v4 drift; `-o marcjson` produces 4160 authority records
(leader/06 z, 0 invalid). The misnamed "v5" file is all v4 IRIs with a v3/v4
inScheme split -- now visible in the summary header.

## Follow-ups

- Streaming for very large vocabularies (LCSH/FAST are millions of concepts);
  whole-graph parse is fine for homosaurus-sized files.
- MARC authority -> SKOS (reverse) is out of scope here.