package rdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readTestdata reads a file from the package testdata directory, returning nil
// when it is absent so tests can skip.
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		return nil
	}
	return data
}

var ntPrefixes = map[string]string{
	"bf":   "http://id.loc.gov/ontologies/bibframe/",
	"rdf":  NS,
	"rdfs": "http://www.w3.org/2000/01/rdf-schema#",
}

func equalTriples(a, b *Graph) bool {
	return strings.Join(tripleStrings(a), "\n") == strings.Join(tripleStrings(b), "\n")
}

// TestNTriplesRoundTrip and TestTurtleRoundTrip serialize a graph (all IRI
// subjects, so blank-node labelling does not interfere) and parse it back,
// expecting the same triples.
func TestNTriplesRoundTrip(t *testing.T) {
	g, _ := ParseRDFXML([]byte(sampleXML))
	back, err := ParseNTriples(g.NTriples())
	if err != nil {
		t.Fatal(err)
	}
	if !equalTriples(g, back) {
		t.Errorf("N-Triples round-trip differs:\n%s", g.NTriples())
	}
}

func TestTurtleRoundTrip(t *testing.T) {
	g, _ := ParseRDFXML([]byte(sampleXML))
	ttl := g.Turtle(ntPrefixes)
	back, err := ParseTurtle(ttl)
	if err != nil {
		t.Fatalf("%v\n%s", err, ttl)
	}
	if !equalTriples(g, back) {
		t.Errorf("Turtle round-trip differs:\n%s", ttl)
	}
}

const turtleSample = `@prefix ex: <http://ex/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
# a comment
ex:s a ex:Thing ;
  ex:name "hi"@en ;
  ex:n 5 ;
  ex:f 1.5 ;
  ex:flag true ;
  ex:ref ex:other , <http://ex/o2> ;
  ex:nested [ ex:p "x" ] ;
  ex:list ( "a" "b" ) ;
  ex:long """multi
line""" .`

// TestTurtleFeatures exercises the Turtle constructs: the `a` keyword, prefixed
// names, language/typed/numeric/boolean literals, object lists, blank-node
// property lists, collections and triple-quoted strings.
func TestTurtleFeatures(t *testing.T) {
	g, err := ParseTurtle([]byte(turtleSample))
	if err != nil {
		t.Fatal(err)
	}
	s := NewIRI("http://ex/s")
	if !g.HasType(s, "http://ex/Thing") {
		t.Error("`a` keyword not parsed as rdf:type")
	}
	if v, _ := g.Object(s, "http://ex/name"); v.Value != "hi" || v.Lang != "en" {
		t.Errorf("lang literal = %+v", v)
	}
	if v, _ := g.Object(s, "http://ex/n"); v.Value != "5" || v.Datatype != xsdInteger {
		t.Errorf("integer = %+v", v)
	}
	if v, _ := g.Object(s, "http://ex/f"); v.Value != "1.5" || v.Datatype != xsdDecimal {
		t.Errorf("decimal = %+v", v)
	}
	if v, _ := g.Object(s, "http://ex/flag"); v.Value != "true" || v.Datatype != xsdBoolean {
		t.Errorf("boolean = %+v", v)
	}
	if refs := g.Objects(s, "http://ex/ref"); len(refs) != 2 {
		t.Errorf("object list = %+v", refs)
	}
	nested, _ := g.Object(s, "http://ex/nested")
	if !nested.IsBlank() {
		t.Errorf("blank property list = %+v", nested)
	} else if v, _ := g.Object(nested, "http://ex/p"); v.Value != "x" {
		t.Errorf("nested property = %+v", v)
	}
	head, _ := g.Object(s, "http://ex/list")
	if first, _ := g.Object(head, FirstIRI); first.Value != "a" {
		t.Errorf("collection head = %+v", first)
	}
	if v, _ := g.Object(s, "http://ex/long"); v.Value != "multi\nline" {
		t.Errorf("triple-quoted = %q", v.Value)
	}
}

// TestCrossFormatRealLoC parses real LoC N-Triples, reserializes it to each
// format, and confirms the triple count survives every round trip.
func TestCrossFormatRealLoC(t *testing.T) {
	data := readTestdata(t, "loc-sample.nt")
	if data == nil {
		t.Skip("no loc-sample.nt")
	}
	g, err := ParseNTriples(data)
	if err != nil {
		t.Fatal(err)
	}
	n := len(g.Triples)
	if n < 100 {
		t.Fatalf("expected a large graph, got %d triples", n)
	}
	for name, round := range map[string]func() (*Graph, error){
		"ntriples": func() (*Graph, error) { return ParseNTriples(g.NTriples()) },
		"turtle":   func() (*Graph, error) { return ParseTurtle(g.Turtle(ntPrefixes)) },
	} {
		back, err := round()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(back.Triples) != n {
			t.Errorf("%s: triple count %d -> %d", name, n, len(back.Triples))
		}
	}
}

// TestSerializationEscapes round-trips literals with control characters, quotes,
// backslashes and non-ASCII through both N-Triples and Turtle, covering the
// escape and unescape paths and datatype/language suffixes.
func TestSerializationEscapes(t *testing.T) {
	g := &Graph{}
	s := NewIRI("http://ex/s")
	val := "tab\tnewline\nquote\"back\\ctrl\x01accenté"
	g.Add(s, NewIRI("http://ex/p"), NewLiteral(val, "", ""))
	g.Add(s, NewIRI("http://ex/typed"), NewLiteral("42", "", "http://ex/int"))
	g.Add(s, NewIRI("http://ex/lang"), NewLiteral("bonjour", "fr", ""))

	for name, back := range map[string]*Graph{
		"ntriples": mustParse(t, ParseNTriples, g.NTriples()),
		"turtle":   mustParse(t, ParseTurtle, g.Turtle(map[string]string{"ex": "http://ex/"})),
	} {
		if v, _ := back.Object(s, "http://ex/p"); v.Value != val {
			t.Errorf("%s: literal = %q, want %q", name, v.Value, val)
		}
		if v, _ := back.Object(s, "http://ex/typed"); v.Datatype != "http://ex/int" {
			t.Errorf("%s: datatype = %q", name, v.Datatype)
		}
		if v, _ := back.Object(s, "http://ex/lang"); v.Lang != "fr" {
			t.Errorf("%s: lang = %q", name, v.Lang)
		}
	}
}

func mustParse(t *testing.T, fn func([]byte) (*Graph, error), data []byte) *Graph {
	t.Helper()
	g, err := fn(data)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, data)
	}
	return g
}

// TestTurtleSPARQLAndNQuads covers SPARQL-style PREFIX/BASE directives and the
// N-Quads tolerance (an ignored fourth term).
func TestTurtleSPARQLAndNQuads(t *testing.T) {
	g, err := ParseTurtle([]byte("PREFIX ex: <http://ex/>\nBASE <http://base/>\nex:s ex:p ex:o ."))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Object(NewIRI("http://ex/s"), "http://ex/p"); !ok {
		t.Error("SPARQL PREFIX not applied")
	}
	q, _ := ParseNTriples([]byte("<http://ex/s> <http://ex/p> <http://ex/o> <http://ex/g> .\n"))
	if len(q.Triples) != 1 || q.Triples[0].O.Value != "http://ex/o" {
		t.Errorf("N-Quads tolerance: %+v", q.Triples)
	}
}

// TestTurtleMalformed checks the parser reports an error (rather than panicking or
// silently succeeding) on truncated input.
func TestTurtleMalformed(t *testing.T) {
	for _, bad := range []string{"@prefix ex: ", "ex:s ex:p", "<http://ex/s>", "@bogus ."} {
		if _, err := ParseTurtle([]byte(bad)); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func FuzzNTriples(f *testing.F) {
	f.Add([]byte("<http://ex/s> <http://ex/p> \"o\" .\n"))
	f.Add([]byte("_:b <http://ex/p> <http://ex/o> ."))
	f.Fuzz(func(t *testing.T, data []byte) {
		g, err := ParseNTriples(data)
		if err != nil {
			return
		}
		wellFormed(t, g)
	})
}

func FuzzTurtle(f *testing.F) {
	f.Add([]byte(turtleSample))
	f.Add([]byte("@prefix ex: <http://ex/> . ex:s ex:p ex:o ."))
	f.Fuzz(func(t *testing.T, data []byte) {
		g, err := ParseTurtle(data)
		if err != nil {
			return
		}
		wellFormed(t, g)
	})
}
