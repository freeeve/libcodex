package rdf

import (
	"io"
	"strings"
	"testing"
)

// TestNQuadsRoundTrip serializes an IRI-only dataset spanning several graphs and
// the default graph, then reparses it, requiring the quads to survive exactly.
func TestNQuadsRoundTrip(t *testing.T) {
	d := &Dataset{}
	d.Add(NewIRI("u:s1"), NewIRI("u:p"), NewIRI("u:o1"), NewIRI("u:g1"))
	d.Add(NewIRI("u:s2"), NewIRI("u:p"), NewLiteral("plain", "", ""), NewIRI("u:g1"))
	d.Add(NewIRI("u:s3"), NewIRI("u:p"), NewLiteral("hi", "en", ""), NewIRI("u:g2"))
	d.Add(NewIRI("u:s4"), NewIRI("u:p"), NewIRI("u:o4"), Term{}) // default graph

	got, err := ParseNQuads(d.NQuads())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Quads) != len(d.Quads) {
		t.Fatalf("round-trip changed quad count: got %d, want %d\n%s", len(got.Quads), len(d.Quads), d.NQuads())
	}
	for i, q := range got.Quads {
		if q != d.Quads[i] {
			t.Errorf("quad %d differs:\n got  %+v\n want %+v", i, q, d.Quads[i])
		}
	}
}

// TestGraphNQuadsProvenance checks Graph.NQuads tags every triple with the given
// provenance graph term, and that the default graph omits the fourth term.
func TestGraphNQuadsProvenance(t *testing.T) {
	g := &Graph{}
	g.Add(NewIRI("u:s"), NewIRI("u:p"), NewIRI("u:o"))
	g.Add(NewIRI("u:s"), NewIRI("u:p2"), NewLiteral("v", "", ""))

	prov := NewIRI("urn:prov:batch-7")
	out := g.NQuads(prov)
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasSuffix(line, "<urn:prov:batch-7> .") {
			t.Errorf("line not tagged with provenance graph: %q", line)
		}
	}

	// The default graph writes three-term lines (no graph term).
	def := g.NQuads(Term{})
	for line := range strings.SplitSeq(strings.TrimSpace(string(def)), "\n") {
		if strings.Contains(line, "urn:prov") {
			t.Errorf("default graph should carry no graph term: %q", line)
		}
	}
	ds, _ := ParseNQuads(def)
	for _, q := range ds.Quads {
		if q.G != (Term{}) {
			t.Errorf("default-graph quad carries a graph term: %+v", q)
		}
	}
}

// TestNQuadsEncoderBlankScope verifies one encoder keeps blank-node labels unique
// across graphs, so two graphs that each label a blank the same do not merge —
// the property that lets records stream into one dataset safely.
func TestNQuadsEncoderBlankScope(t *testing.T) {
	g1 := &Graph{}
	g1.Add(NewBlank("x"), NewIRI("u:p"), NewIRI("u:o1"))
	g2 := &Graph{}
	g2.Add(NewBlank("x"), NewIRI("u:p"), NewIRI("u:o2"))

	var e NQuadsEncoder
	var b []byte
	b = e.AppendGraph(b, g1, NewIRI("u:g1"))
	b = e.AppendGraph(b, g2, NewIRI("u:g2"))

	ds, _ := ParseNQuads(b)
	if len(ds.Quads) != 2 {
		t.Fatalf("got %d quads, want 2\n%s", len(ds.Quads), b)
	}
	if ds.Quads[0].S == ds.Quads[1].S {
		t.Errorf("distinct blank subjects merged to %v across graphs:\n%s", ds.Quads[0].S, b)
	}
}

// TestDatasetGraphExtraction checks Graph and Graphs pull a named graph's triples
// back out — the read side of provenance.
func TestDatasetGraphExtraction(t *testing.T) {
	d := &Dataset{}
	d.Add(NewIRI("u:s1"), NewIRI("u:p"), NewIRI("u:o1"), NewIRI("u:g1"))
	d.Add(NewIRI("u:s2"), NewIRI("u:p"), NewIRI("u:o2"), NewIRI("u:g2"))
	d.Add(NewIRI("u:s3"), NewIRI("u:p"), NewIRI("u:o3"), NewIRI("u:g1"))

	if gs := d.Graphs(); len(gs) != 2 || gs[0].Value != "u:g1" || gs[1].Value != "u:g2" {
		t.Fatalf("Graphs() = %+v, want [u:g1 u:g2]", gs)
	}
	g1 := d.Graph(NewIRI("u:g1"))
	if len(g1.Triples) != 2 {
		t.Fatalf("graph u:g1 has %d triples, want 2", len(g1.Triples))
	}
}

// TestStreamingDecodeQuad checks the streaming decoder preserves N-Quads graph
// terms through DecodeQuad while Decode still projects to triples.
func TestStreamingDecodeQuad(t *testing.T) {
	doc := "<u:s> <u:p> <u:o> <u:g> .\n" +
		"<u:s2> <u:p> \"v\" .\n" // default graph

	// DecodeQuad keeps the graph.
	d := NewDecoder(strings.NewReader(doc), NQuads)
	var quads []Quad
	for {
		q, err := d.DecodeQuad()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		quads = append(quads, q)
	}
	if len(quads) != 2 {
		t.Fatalf("got %d quads, want 2", len(quads))
	}
	if quads[0].G.Value != "u:g" {
		t.Errorf("first quad graph = %+v, want u:g", quads[0].G)
	}
	if quads[1].G != (Term{}) {
		t.Errorf("second quad should be default graph, got %+v", quads[1].G)
	}

	// Decode drops the graph (unchanged behavior).
	tr, _ := NewDecoder(strings.NewReader(doc), NQuads).Decode()
	if tr.O.Value != "u:o" || (tr != Triple{tr.S, tr.P, tr.O}) {
		t.Errorf("Decode triple = %+v", tr)
	}
}
