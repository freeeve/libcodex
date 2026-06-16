# 032 — Conformance: BIBFRAME 2.0

Verify the RDF/XML and JSON-LD parse as RDF and use only terms defined in the
BIBFRAME ontology.

## References
- BIBFRAME 2.0 vocabulary: http://id.loc.gov/ontologies/bibframe/
- LoC `marc2bibframe2` conversion specs (for mapping patterns).
- RDF/XML and JSON-LD 1.1 syntax specs.

## Checks
- Every class (`bf:Work`, `bf:Instance`, `bf:Title`, `bf:Person`, …) and property
  (`bf:title`, `bf:contribution`, `bf:provisionActivity`, `bf:instanceOf`, …) used
  exists in the BIBFRAME ontology; `bflc:` terms exist in the BIBFRAME-LC vocab.
- RDF/XML is well-formed and produces the intended triples; JSON-LD expands to the
  same graph.
- Node IRIs are well-formed; `bf:instanceOf`/`bf:hasInstance` link consistently.
- Mapping choices align with marc2bibframe2 for the covered fields.

## Verification
- Parse RDF/XML with an RDF tool (`rapper`/`riot`) and confirm the triple set.
- Expand the JSON-LD (a JSON-LD processor) and diff the graph against the RDF/XML.
- Cross-check term names against the ontology term list.

## Acceptance
- Both serializations parse as RDF to the same graph using only defined terms;
  mapping cited against marc2bibframe2.

## Depends on
- bibframe (task 015).
