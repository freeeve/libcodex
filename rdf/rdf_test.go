package rdf

import (
	"sort"
	"strings"
	"testing"
)

const bf = "http://id.loc.gov/ontologies/bibframe/"

// tripleStrings renders a graph's triples as comparable strings (kind-tagged so a
// literal "x" and an IRI "x" never collide), sorted.
func tripleStrings(g *Graph) []string {
	out := make([]string, len(g.Triples))
	for i, t := range g.Triples {
		out[i] = termString(t.S) + " " + termString(t.P) + " " + termString(t.O)
	}
	sort.Strings(out)
	return out
}

func termString(t Term) string {
	switch t.Kind {
	case IRI:
		return "<" + t.Value + ">"
	case Blank:
		return "_:" + t.Value
	default:
		s := `"` + t.Value + `"`
		if t.Lang != "" {
			s += "@" + t.Lang
		}
		return s
	}
}

const sampleXML = `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
         xmlns:bf="http://id.loc.gov/ontologies/bibframe/">
  <bf:Work rdf:about="http://ex/w1">
    <rdf:type rdf:resource="http://id.loc.gov/ontologies/bibframe/Text"/>
    <bf:title>
      <bf:Title rdf:about="http://ex/t1">
        <bf:mainTitle>Hello &amp; World</bf:mainTitle>
        <bf:subtitle xml:lang="en">a tale</bf:subtitle>
      </bf:Title>
    </bf:title>
    <bf:hasInstance rdf:resource="http://ex/i1"/>
  </bf:Work>
</rdf:RDF>`

// The same graph as sampleXML, expressed in compact JSON-LD.
const sampleJSONLD = `{
  "@context": {"bf": "http://id.loc.gov/ontologies/bibframe/",
               "rdfs": "http://www.w3.org/2000/01/rdf-schema#"},
  "@graph": [
    {"@id": "http://ex/w1", "@type": ["bf:Work", "bf:Text"],
     "bf:title": {"@id": "http://ex/t1", "@type": "bf:Title",
                  "bf:mainTitle": "Hello & World",
                  "bf:subtitle": {"@value": "a tale", "@language": "en"}},
     "bf:hasInstance": {"@id": "http://ex/i1"}}
  ]
}`

// TestParsersAgree confirms the RDF/XML and JSON-LD parsers produce the same
// triples for the same graph, the property the reverse crosswalk relies on.
func TestParsersAgree(t *testing.T) {
	gx, err := ParseRDFXML([]byte(sampleXML))
	if err != nil {
		t.Fatalf("RDF/XML: %v", err)
	}
	gj, err := ParseJSONLD([]byte(sampleJSONLD))
	if err != nil {
		t.Fatalf("JSON-LD: %v", err)
	}
	x, j := tripleStrings(gx), tripleStrings(gj)
	if strings.Join(x, "\n") != strings.Join(j, "\n") {
		t.Errorf("parsers disagree:\nRDF/XML:\n%s\n\nJSON-LD:\n%s", strings.Join(x, "\n"), strings.Join(j, "\n"))
	}
}

// TestRDFXMLStructure spot-checks individual triples and the graph helpers.
func TestRDFXMLStructure(t *testing.T) {
	g, err := ParseRDFXML([]byte(sampleXML))
	if err != nil {
		t.Fatal(err)
	}
	w := NewIRI("http://ex/w1")
	if !g.HasType(w, bf+"Work") || !g.HasType(w, bf+"Text") {
		t.Error("Work missing expected types")
	}
	title, ok := g.Object(w, bf+"title")
	if !ok || title != NewIRI("http://ex/t1") {
		t.Fatalf("bf:title = %+v, %v", title, ok)
	}
	if mt := g.Objects(title, bf+"mainTitle"); len(mt) != 1 || mt[0].Value != "Hello & World" {
		t.Errorf("mainTitle = %+v", mt)
	}
	sub := g.Objects(title, bf+"subtitle")
	if len(sub) != 1 || sub[0].Value != "a tale" || sub[0].Lang != "en" {
		t.Errorf("subtitle = %+v", sub)
	}
	if works := g.SubjectsOfType(bf + "Work"); len(works) != 1 || works[0] != w {
		t.Errorf("SubjectsOfType(Work) = %+v", works)
	}
}

// TestJSONLDList checks @list expands to an RDF collection.
func TestJSONLDList(t *testing.T) {
	doc := `{"@context":{"ex":"http://ex/"},
	         "@id":"http://ex/s","ex:items":{"@list":["a","b"]}}`
	g, err := ParseJSONLD([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	head, ok := g.Object(NewIRI("http://ex/s"), "http://ex/items")
	if !ok || !head.IsBlank() {
		t.Fatalf("list head = %+v, %v", head, ok)
	}
	first, _ := g.Object(head, FirstIRI)
	if first.Value != "a" {
		t.Errorf("first = %+v", first)
	}
	rest, _ := g.Object(head, RestIRI)
	second, _ := g.Object(rest, FirstIRI)
	if second.Value != "b" {
		t.Errorf("second = %+v", second)
	}
	if tail, _ := g.Object(rest, RestIRI); tail.Value != NilIRI {
		t.Errorf("tail = %+v, want rdf:nil", tail)
	}
}

// TestParseTypeResource checks rdf:parseType="Resource" yields a blank node whose
// nested properties attach to it.
func TestParseTypeResource(t *testing.T) {
	doc := `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:ex="http://ex/">
	  <ex:Thing rdf:about="http://ex/s">
	    <ex:detail rdf:parseType="Resource"><ex:name>n</ex:name></ex:detail>
	  </ex:Thing></rdf:RDF>`
	g, err := ParseRDFXML([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	d, ok := g.Object(NewIRI("http://ex/s"), "http://ex/detail")
	if !ok || !d.IsBlank() {
		t.Fatalf("detail = %+v, %v", d, ok)
	}
	if n, _ := g.Object(d, "http://ex/name"); n.Value != "n" {
		t.Errorf("name = %+v", n)
	}
}

// wellFormed asserts an RDF graph invariant the parsers must always uphold: every
// triple has a non-literal subject and an IRI predicate. A literal subject or a
// blank/literal predicate would be a parser bug.
func wellFormed(t *testing.T, g *Graph) {
	for _, tr := range g.Triples {
		if tr.S.IsLiteral() {
			t.Fatalf("triple has a literal subject: %+v", tr)
		}
		if !tr.P.IsIRI() {
			t.Fatalf("triple predicate is not an IRI: %+v", tr)
		}
	}
}

// FuzzRDFXML and FuzzJSONLD assert the parsers never panic and always produce a
// well-formed graph.
func FuzzRDFXML(f *testing.F) {
	f.Add([]byte(sampleXML))
	f.Add([]byte(coverXML))
	f.Add([]byte("<rdf:RDF"))
	f.Fuzz(func(t *testing.T, data []byte) {
		g, err := ParseRDFXML(data)
		if err != nil {
			return
		}
		wellFormed(t, g)
	})
}

func FuzzJSONLD(f *testing.F) {
	f.Add([]byte(sampleJSONLD))
	f.Add([]byte(coverJSONLD))
	f.Add([]byte("{"))
	f.Fuzz(func(t *testing.T, data []byte) {
		g, err := ParseJSONLD(data)
		if err != nil {
			return
		}
		wellFormed(t, g)
	})
}
