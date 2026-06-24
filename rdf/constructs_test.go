package rdf

import "testing"

const coverXML = `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:ex="http://ex/">
  <rdf:Description rdf:about="http://ex/s" ex:tag="lit">
    <ex:ref rdf:nodeID="n1"/>
    <ex:num rdf:datatype="http://www.w3.org/2001/XMLSchema#integer">5</ex:num>
  </rdf:Description>
  <ex:Thing rdf:nodeID="n1"><ex:name>blank</ex:name></ex:Thing>
  <ex:Anon rdf:ID="frag"><ex:p>y</ex:p></ex:Anon>
</rdf:RDF>`

// TestRDFXMLConstructs covers rdf:Description, property attributes, rdf:nodeID
// (subject and object), rdf:ID, and typed literals.
func TestRDFXMLConstructs(t *testing.T) {
	g, err := ParseRDFXML([]byte(coverXML))
	if err != nil {
		t.Fatal(err)
	}
	s := NewIRI("http://ex/s")

	if len(g.Objects(s, TypeIRI)) != 0 {
		t.Error("rdf:Description should not emit a type triple")
	}
	if tag := g.Objects(s, "http://ex/tag"); len(tag) != 1 || !tag[0].IsLiteral() || tag[0].Value != "lit" {
		t.Errorf("property attribute = %+v", tag)
	}
	ref, _ := g.Object(s, "http://ex/ref")
	if !ref.IsBlank() || ref.Value != "n1" {
		t.Errorf("nodeID object = %+v", ref)
	}
	num, _ := g.Object(s, "http://ex/num")
	if num.Value != "5" || num.Datatype != "http://www.w3.org/2001/XMLSchema#integer" {
		t.Errorf("typed literal = %+v", num)
	}
	if !g.HasType(ref, "http://ex/Thing") {
		t.Error("nodeID subject lost its type")
	}
	if name := g.Objects(ref, "http://ex/name"); len(name) != 1 || name[0].Value != "blank" {
		t.Errorf("nodeID subject name = %+v", name)
	}
	frag := NewIRI("#frag")
	if !g.HasType(frag, "http://ex/Anon") {
		t.Error("rdf:ID subject missing")
	}
}

const coverJSONLD = `{
  "@context": [
    {"ex": "http://ex/"},
    {"link": {"@id": "http://ex/link", "@type": "@id"}, "name": "http://ex/name"}
  ],
  "@graph": [
    {"@id": "_:b0", "@type": "ex:Thing",
     "name": "hi",
     "ex:n": 5, "ex:f": 1.5, "ex:flag": true,
     "link": "http://ex/target",
     "ex:typed": {"@value": 7, "@type": "ex:int"},
     "ex:vals": {"@list": [1, 2]}}
  ]
}`

// TestJSONLDConstructs covers array contexts, term definitions with @id/@type
// coercion, blank-node @id, numeric and boolean values, and typed @value objects.
func TestJSONLDConstructs(t *testing.T) {
	g, err := ParseJSONLD([]byte(coverJSONLD))
	if err != nil {
		t.Fatal(err)
	}
	s := NewBlank("b0")

	if !g.HasType(s, "http://ex/Thing") {
		t.Error("blank-node subject lost its type")
	}
	if v := g.Objects(s, "http://ex/name"); len(v) != 1 || v[0].Value != "hi" {
		t.Errorf("term-defined predicate = %+v", v)
	}
	if v, _ := g.Object(s, "http://ex/n"); v.Value != "5" {
		t.Errorf("integer = %+v", v)
	}
	if v, _ := g.Object(s, "http://ex/f"); v.Value != "1.5" {
		t.Errorf("float = %+v", v)
	}
	if v, _ := g.Object(s, "http://ex/flag"); v.Value != "true" {
		t.Errorf("bool = %+v", v)
	}
	// "link" is coerced to @id, so the string is an IRI reference, not a literal.
	if v, _ := g.Object(s, "http://ex/link"); !v.IsIRI() || v.Value != "http://ex/target" {
		t.Errorf("coerced IRI = %+v", v)
	}
	if v, _ := g.Object(s, "http://ex/typed"); v.Value != "7" || v.Datatype != "http://ex/int" {
		t.Errorf("typed @value = %+v", v)
	}
}

// TestTermHelpers exercises the small term/graph helpers directly.
func TestTermHelpers(t *testing.T) {
	if !NewIRI("x").IsIRI() || NewBlank("b").IsBlank() == false || !NewLiteral("v", "", "").IsLiteral() {
		t.Error("kind predicates")
	}
	cases := map[string]string{
		"http://ex/path/Leaf": "Leaf",
		"http://ex/ns#Frag":   "Frag",
		"bare":                "bare",
	}
	for in, want := range cases {
		if got := LocalName(in); got != want {
			t.Errorf("LocalName(%q) = %q, want %q", in, got, want)
		}
	}
	var g Graph
	if _, ok := g.Object(NewIRI("none"), "p"); ok {
		t.Error("Object on empty graph should report false")
	}
	if g.HasType(NewIRI("none"), "T") {
		t.Error("HasType on empty graph should be false")
	}
}
