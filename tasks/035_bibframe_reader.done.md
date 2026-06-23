# 035 — BIBFRAME reading (general RDF importer)

Decision: build a general BIBFRAME reader that handles arbitrary BIBFRAME from
any source, including a hand-rolled RDF parser to keep the zero-dependency rule.

## Pieces
1. `internal/rdf` — a stdlib RDF data model (Term: IRI/blank/literal; Triple;
   Graph with query helpers) and two parsers producing triples:
   - RDF/XML (encoding/xml): striped syntax, typed nodes, rdf:about/nodeID/ID,
     rdf:resource, rdf:Description, property literals with rdf:datatype/xml:lang,
     base resolution. Common subset first; document unsupported exotica
     (containers, reification, parseType=Literal).
   - JSON-LD (encoding/json): @context prefix map, @graph, @id/@type, nested
     nodes, @value/@language/@type, @list. Inline contexts; no remote fetch.
2. `bibframe` reader — interpret the triples as a BIBFRAME graph (find bf:Work /
   bf:Instance and their bf: properties) and reverse-crosswalk the common fields
   to a `codex.Record` (245 from bf:title, 1xx/7xx from bf:contribution, 020/022
   from bf:identifiedBy, 260 from bf:provisionActivity, 6xx from bf:subject, …).
   Surface: `NewReader` / `Decode` / `ReadFile` (format auto-detected or by entry).

## Validation
- Round-trip: Encode(rec) -> Decode -> compare common fields to the original.
- Differential: parse with rdflib (interop harness) and confirm our triples match.
- Read real LoC BIBFRAME samples.
- Fuzz the parsers (never panic on arbitrary input).

## Out of scope
- Full JSON-LD 1.1 processing (remote contexts, framing) and exotic RDF/XML
  (reification, rdf containers) beyond what real BIBFRAME uses.
