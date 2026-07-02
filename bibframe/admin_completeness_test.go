package bibframe

import (
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// TestAdminAssignerAndConventions covers 003 -> bf:assigner on the 001 bf:Local, and
// multiple 040 $e -> bf:DescriptionConventions nodes with vocabulary IRIs (task 069).
func TestAdminAssignerAndConventions(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "12345")).
		AddField(codex.NewControlField("003", "DLC")).
		AddField(codex.NewDataField("040", ' ', ' ', codex.NewSubfield('e', "rda"), codex.NewSubfield('e', "pn"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))

	g := FromRecord(rec)
	if g.Instance.Admin.ControlOrg != "DLC" {
		t.Errorf("ControlOrg = %q, want DLC", g.Instance.Admin.ControlOrg)
	}
	if len(g.Instance.Admin.DescriptionConventions) != 2 {
		t.Errorf("descriptionConventions = %+v, want 2", g.Instance.Admin.DescriptionConventions)
	}

	graph, _ := rdf.ParseNTriples(mustEncodeNT(t, rec))
	// The bf:Local (001) carries a bf:assigner agent with the organizations IRI.
	local := graph.SubjectsOfType(classLocal)
	if len(local) != 1 {
		t.Fatalf("want 1 bf:Local, got %d", len(local))
	}
	assigner, ok := graph.Object(local[0], bfNS+"assigner")
	if !ok {
		t.Fatal("bf:Local has no bf:assigner")
	}
	if !assigner.IsIRI() || assigner.Value != "http://id.loc.gov/vocabulary/organizations/dlc" {
		t.Errorf("assigner = %+v, want organizations/dlc IRI", assigner)
	}
	if v, _ := graph.Literal(assigner, bfNS+"code"); v != "DLC" {
		t.Errorf("assigner bf:code = %q, want DLC", v)
	}
	// Two bf:DescriptionConventions nodes, one carrying the rda vocabulary IRI.
	if n := len(graph.SubjectsOfType(bfNS + "DescriptionConventions")); n != 2 {
		t.Errorf("want 2 DescriptionConventions nodes, got %d", n)
	}
	var sawRDA bool
	for _, o := range graph.Objects(graph.SubjectsOfType(classAdminMetadata)[0], bfNS+"descriptionConventions") {
		if o.Value == "http://id.loc.gov/vocabulary/descriptionConventions/rda" {
			sawRDA = true
		}
	}
	if !sawRDA {
		t.Error("no descriptionConventions node with the rda vocabulary IRI")
	}
}

// TestChangeDateTyped confirms 005 is emitted as an xsd:dateTime typed literal in
// all serializations and survives a JSON-LD round-trip (task 069).
func TestChangeDateTyped(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "12345")).
		AddField(codex.NewControlField("005", "19931204093000.0")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))

	x, _ := Encode(rec)
	if !strings.Contains(string(x), `<bf:changeDate rdf:datatype="http://www.w3.org/2001/XMLSchema#dateTime">1993-12-04T09:30:00</bf:changeDate>`) {
		t.Errorf("RDF/XML changeDate not xsd:dateTime typed:\n%s", x)
	}
	j, _ := EncodeJSONLD(rec)
	if !strings.Contains(string(j), `"bf:changeDate":{"@value":"1993-12-04T09:30:00","@type":"http://www.w3.org/2001/XMLSchema#dateTime"}`) {
		t.Errorf("JSON-LD changeDate not xsd:dateTime typed:\n%s", j)
	}
	// The datatype survives parsing in every serialization.
	for name, b := range map[string][]byte{"rdfxml": x, "jsonld": j} {
		g, err := parseGraph(b)
		if err != nil {
			t.Fatalf("%s parse: %v", name, err)
		}
		am := g.SubjectsOfType(classAdminMetadata)
		if len(am) != 1 {
			t.Fatalf("%s: want 1 AdminMetadata, got %d", name, len(am))
		}
		found := false
		for _, o := range g.Objects(am[0], bfNS+"changeDate") {
			if o.Datatype == xsdDateTime {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: changeDate lost its xsd:dateTime datatype", name)
		}
	}
}
