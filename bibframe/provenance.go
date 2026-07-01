package bibframe

import "github.com/freeeve/libcodex/rdf"

const (
	provNS      = "http://www.w3.org/ns/prov#"
	xsdDateTime = "http://www.w3.org/2001/XMLSchema#dateTime"
)

// Provenance returns PROV-O triples describing a named graph as an entity that
// was generated from a source, attributed to an agent, at a time — the
// nanopublication / W3C PROV pattern of asserting provenance with the data-graph
// IRI itself as the subject, kept distinct from the data it describes. This
// complements the in-graph bf:AdminMetadata: AdminMetadata travels inside every
// serialization for BIBFRAME consumers, while these triples give an N-Quads
// dataset a separate, machine-readable provenance record about each named graph.
//
// Zero-value source or agent terms and an empty generatedAt are omitted.
// Serialize the result into a provenance graph alongside the data graph named by
// graph, e.g. with an rdf.Encoder:
//
//	var enc rdf.Encoder
//	buf = enc.AppendNQuads(buf, dataGraph, g)                         // the record, in graph g
//	buf = enc.AppendNQuads(buf, bibframe.Provenance(g, src, who, ts), provGraph) // its provenance
func Provenance(graph, source, agent rdf.Term, generatedAt string) *rdf.Graph {
	g := &rdf.Graph{}
	g.Add(graph, rdf.NewIRI(rdfNS+"type"), rdf.NewIRI(provNS+"Entity"))
	if source != (rdf.Term{}) {
		g.Add(graph, rdf.NewIRI(provNS+"wasDerivedFrom"), source)
	}
	if agent != (rdf.Term{}) {
		g.Add(graph, rdf.NewIRI(provNS+"wasAttributedTo"), agent)
	}
	if generatedAt != "" {
		g.Add(graph, rdf.NewIRI(provNS+"generatedAtTime"), rdf.NewLiteral(generatedAt, "", xsdDateTime))
	}
	return g
}
