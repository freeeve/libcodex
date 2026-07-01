package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// TestAdminMetadataConsistency checks the bf:AdminMetadata node is emitted, and
// that the RDF/XML, JSON-LD and N-Triples serializations still denote the same
// graph (equal triple counts) with it present.
func TestAdminMetadataConsistency(t *testing.T) {
	rec := sample()
	xml, _ := Encode(rec)
	jsonld, _ := EncodeJSONLD(rec)
	nt, _ := EncodeNTriples(rec)
	gx, _ := rdf.ParseRDFXML(xml)
	gj, _ := rdf.ParseJSONLD(jsonld)
	gn, _ := rdf.ParseNTriples(nt)
	if len(gx.Triples) != len(gn.Triples) || len(gj.Triples) != len(gn.Triples) {
		t.Fatalf("serializations disagree on triple count: xml=%d jsonld=%d nt=%d",
			len(gx.Triples), len(gj.Triples), len(gn.Triples))
	}
	if n := len(gn.SubjectsOfType(bfNS + "AdminMetadata")); n != 1 {
		t.Errorf("want 1 bf:AdminMetadata node, got %d", n)
	}
	if n := len(gn.SubjectsOfType(bfNS + "GenerationProcess")); n != 1 {
		t.Errorf("want 1 bf:GenerationProcess node, got %d", n)
	}
}

// TestAdminMetadataFields checks the control number, change date and description
// conventions flow from MARC 001/005/040 into the AdminMetadata node.
func TestAdminMetadataFields(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "ocm12345")).
		AddField(codex.NewControlField("005", "19931204093000.0")).
		AddField(codex.NewDataField("040", ' ', ' ', codex.NewSubfield('a', "DLC"), codex.NewSubfield('e', "rda"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))

	g, _ := rdf.ParseNTriples(mustEncodeNT(t, rec))
	am := g.SubjectsOfType(bfNS + "AdminMetadata")
	if len(am) != 1 {
		t.Fatalf("want 1 AdminMetadata node, got %d", len(am))
	}
	node := am[0]
	if v, _ := g.Literal(node, bfNS+"changeDate"); v != "1993-12-04T09:30:00" {
		t.Errorf("changeDate = %q, want 1993-12-04T09:30:00", v)
	}
	if v, _ := g.Literal(node, bfNS+"descriptionConventions"); v != "rda" {
		t.Errorf("descriptionConventions = %q, want rda", v)
	}
	// The control number is on a nested bf:Local identifier.
	id, ok := g.Object(node, bfNS+"identifiedBy")
	if !ok {
		t.Fatal("AdminMetadata has no bf:identifiedBy")
	}
	if v, _ := g.Literal(id, rdfNS+"value"); v != "ocm12345" {
		t.Errorf("control number = %q, want ocm12345", v)
	}
}

// TestProvenance checks the PROV-O helper asserts provenance with the data-graph
// IRI as subject, omitting zero-value inputs.
func TestProvenance(t *testing.T) {
	graph := rdf.NewIRI("#rec-1")
	src := rdf.NewIRI("urn:marc:rec-1")
	agent := rdf.NewIRI("https://example.org/libcodex")
	g := Provenance(graph, src, agent, "2026-07-01T00:00:00")

	if !g.HasType(graph, provNS+"Entity") {
		t.Error("graph is not typed prov:Entity")
	}
	if o, _ := g.Object(graph, provNS+"wasDerivedFrom"); o != src {
		t.Errorf("wasDerivedFrom = %+v, want %+v", o, src)
	}
	if o, _ := g.Object(graph, provNS+"wasAttributedTo"); o != agent {
		t.Errorf("wasAttributedTo = %+v, want %+v", o, agent)
	}
	if v, _ := g.Literal(graph, provNS+"generatedAtTime"); v != "2026-07-01T00:00:00" {
		t.Errorf("generatedAtTime = %q", v)
	}

	// Zero-value source/agent and empty time are omitted.
	bare := Provenance(graph, rdf.Term{}, rdf.Term{}, "")
	if len(bare.Triples) != 1 { // only the rdf:type prov:Entity
		t.Errorf("bare provenance has %d triples, want 1", len(bare.Triples))
	}
}

func mustEncodeNT(t *testing.T, r *codex.Record) []byte {
	t.Helper()
	b, err := EncodeNTriples(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
