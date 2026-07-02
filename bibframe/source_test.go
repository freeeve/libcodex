package bibframe

import (
	"reflect"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// sourcedRecord carries a source-qualified identifier (024 $2) and classification
// (072 $2) — the OverDrive/BISAC shapes task 037 targets.
func sourcedRecord() *codex.Record {
	return codex.NewRecord().
		AddField(codex.NewControlField("001", "od-123")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "A Sourced Title"))).
		AddField(codex.NewDataField("024", '7', ' ', codex.NewSubfield('a', "OD-RESERVE-42"), codex.NewSubfield('2', "overdrive"))).
		AddField(codex.NewDataField("072", ' ', '7', codex.NewSubfield('a', "FIC000000"), codex.NewSubfield('2', "bisacsh")))
}

// TestSourceEmitted checks bf:source is emitted for a sourced identifier and
// classification in every serialization, all four denoting the same graph, and
// that the source labels survive.
func TestSourceEmitted(t *testing.T) {
	rec := sourcedRecord()
	x, _ := Encode(rec)
	j, _ := EncodeJSONLD(rec)
	nt, _ := EncodeNTriples(rec)
	ttl, _ := EncodeTurtle(rec)

	parsers := map[string]func() (*rdf.Graph, error){
		"rdfxml":   func() (*rdf.Graph, error) { return rdf.ParseRDFXML(x) },
		"jsonld":   func() (*rdf.Graph, error) { return rdf.ParseJSONLD(j) },
		"ntriples": func() (*rdf.Graph, error) { return rdf.ParseNTriples(nt) },
		"turtle":   func() (*rdf.Graph, error) { return rdf.ParseTurtle(ttl) },
	}
	var want []string
	for name, p := range parsers {
		g, err := p()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// Two bf:Source nodes: one on the identifier, one on the classification.
		if n := len(g.SubjectsOfType(classSource)); n != 2 {
			t.Errorf("%s: %d bf:Source nodes, want 2\n%s", name, n, canonGraph(g))
		}
		labels := sourceLabels(g)
		if !labels["overdrive"] || !labels["bisacsh"] {
			t.Errorf("%s: source labels = %v, want overdrive+bisacsh", name, labels)
		}
		if got := canonGraph(g); want == nil {
			want = got
		} else if !reflect.DeepEqual(want, got) {
			t.Errorf("%s graph differs from another serialization", name)
		}
	}
}

// TestSourceOmittedWhenEmpty checks that a record whose access points carry no
// scheme emits no bf:Source node: an 020/050 without $2 and a 650 with a blank
// second indicator (no thesaurus).
func TestSourceOmittedWhenEmpty(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "0786803525"))).
		AddField(codex.NewDataField("050", ' ', '0', codex.NewSubfield('a', "PS3556"))).
		AddField(codex.NewDataField("650", ' ', ' ', codex.NewSubfield('a', "Lesbians")))
	g, _ := rdf.ParseNTriples(mustEncodeNT(t, rec))
	if n := len(g.SubjectsOfType(classSource)); n != 0 {
		t.Errorf("unsourced record emitted %d bf:Source nodes, want 0", n)
	}
}

// sourceLabels returns the set of rdfs:label values on bf:Source nodes.
func sourceLabels(g *rdf.Graph) map[string]bool {
	out := map[string]bool{}
	for _, s := range g.SubjectsOfType(classSource) {
		if v, ok := g.Literal(s, pLabel); ok {
			out[v] = true
		}
	}
	return out
}
