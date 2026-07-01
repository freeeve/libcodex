package rdf

import (
	"bytes"
	"testing"
)

// TestTurtleBaseAbsoluteIRIs checks that @base joins only relative references and
// leaves absolute IRIs -- including non-hierarchical urn:/mailto:/info: forms that
// contain no "://" -- untouched.
func TestTurtleBaseAbsoluteIRIs(t *testing.T) {
	cases := []struct {
		subject, want string
	}{
		{"<urn:isbn:0451450523>", "urn:isbn:0451450523"},
		{"<mailto:a@b.example>", "mailto:a@b.example"},
		{"<info:lccn/2002022641>", "info:lccn/2002022641"},
		{"<rel>", "http://ex/rel"},
		{"<#frag>", "http://ex/#frag"},
	}
	for _, c := range cases {
		doc := "@base <http://ex/> . " + c.subject + ` <http://ex/p> "x" .`
		g, err := ParseTurtle([]byte(doc))
		if err != nil {
			t.Fatalf("ParseTurtle(%s): %v", c.subject, err)
		}
		if len(g.Triples) != 1 || g.Triples[0].S.Value != c.want {
			t.Errorf("subject of %s = %+v, want %q", c.subject, g.Triples, c.want)
		}
	}
}

// TestTurtleRejectsMalformedNumbers checks a bare sign or an exponent with no
// digits is rejected rather than accepted as a literal with an invalid lexical
// form.
func TestTurtleRejectsMalformedNumbers(t *testing.T) {
	for _, bad := range []string{"+", "-", "1e", "1E+", ".e5"} {
		doc := `<http://ex/s> <http://ex/p> ` + bad + " ."
		if _, err := ParseTurtle([]byte(doc)); err == nil {
			t.Errorf("ParseTurtle accepted malformed number %q", bad)
		}
	}
	// A well-formed number still parses.
	if _, err := ParseTurtle([]byte(`<http://ex/s> <http://ex/p> 3.14 .`)); err != nil {
		t.Errorf("ParseTurtle rejected valid decimal: %v", err)
	}
}

// TestJSONLDBlankLabelNoCollision checks a document blank label ("_:j1") cannot
// merge with a generated blank -- the outer node and the anonymous inner node
// stay distinct.
func TestJSONLDBlankLabelNoCollision(t *testing.T) {
	doc := `{"@id":"_:j1","http://ex/p":{"http://ex/q":"x"}}`
	g, err := ParseJSONLD([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	outer, _ := g.Object(NewBlank("uj1"), "http://ex/p")
	if !outer.IsBlank() {
		t.Fatalf("outer node's object is not a blank: %+v", outer)
	}
	if outer.Value == "uj1" {
		t.Error("anonymous inner node collided with the document label _:j1")
	}
	if v, _ := g.Object(outer, "http://ex/q"); v.Value != "x" {
		t.Errorf("inner node lost its property: %+v", v)
	}
}

// TestJSONLDDeterministic checks that parsing the same multi-predicate document
// twice yields the triples in the same order (keys are sorted), so serialized
// output is reproducible.
func TestJSONLDDeterministic(t *testing.T) {
	doc := `{"@id":"http://ex/s","http://ex/c":"3","http://ex/a":"1","http://ex/b":"2"}`
	var first []Triple
	for i := 0; i < 5; i++ {
		g, err := ParseJSONLD([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = g.Triples
			continue
		}
		if len(g.Triples) != len(first) {
			t.Fatalf("run %d: %d triples, first run %d", i, len(g.Triples), len(first))
		}
		for j := range g.Triples {
			if g.Triples[j] != first[j] {
				t.Fatalf("run %d triple %d differs from first run: %+v vs %+v", i, j, g.Triples[j], first[j])
			}
		}
	}
}

// TestJSONLDIntegerDatatype checks a whole JSON number maps to xsd:integer, not
// xsd:double.
func TestJSONLDIntegerDatatype(t *testing.T) {
	g, err := ParseJSONLD([]byte(`{"@id":"http://ex/s","http://ex/n":5,"http://ex/f":1.5}`))
	if err != nil {
		t.Fatal(err)
	}
	n, _ := g.Object(NewIRI("http://ex/s"), "http://ex/n")
	if n.Value != "5" || n.Datatype != xsdInteger {
		t.Errorf("integer = %+v, want 5^^xsd:integer", n)
	}
	fv, _ := g.Object(NewIRI("http://ex/s"), "http://ex/f")
	if fv.Datatype != xsdDouble {
		t.Errorf("fractional = %+v, want xsd:double", fv)
	}
}

// TestNTriplesBlankTrailingDot checks a blank label directly followed by the
// terminating "." (no space) does not swallow the dot into the label.
func TestNTriplesBlankTrailingDot(t *testing.T) {
	g, err := ParseNTriples([]byte("<http://ex/s> <http://ex/p> _:b.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Triples) != 1 || g.Triples[0].O.Value != "b" {
		t.Errorf("object = %+v, want blank label \"b\"", g.Triples)
	}
}

// TestStreamTerminatorNoWhitespace checks the streaming Turtle decoder frames a
// "." terminator that is not followed by whitespace the same as the whole-document
// parser, rather than dropping the following statement.
func TestStreamTerminatorNoWhitespace(t *testing.T) {
	doc := `<http://ex/s> <http://ex/p> <http://ex/o>.<http://ex/s2> <http://ex/p> <http://ex/o2>.`
	g, err := ParseTurtle([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	streamed := drain(NewDecoder(bytes.NewReader([]byte(doc)), Turtle))
	sameTriples(t, "turtle", streamed, g.Triples)
	if len(streamed) != 2 {
		t.Errorf("streamed %d triples, want 2", len(streamed))
	}
}

// TestTurtleBareBlankPropertyList checks that a blankNodePropertyList used as a
// statement subject with no trailing predicate-object list -- "[ a :C ] ." -- is
// accepted, per the Turtle grammar where that predicate-object list is optional.
func TestTurtleBareBlankPropertyList(t *testing.T) {
	doc := "[ a <http://ex/Work> ; <http://ex/p> \"x\" ] .\n"
	g, err := ParseTurtle([]byte(doc))
	if err != nil {
		t.Fatalf("ParseTurtle: %v", err)
	}
	if len(g.Triples) != 2 {
		t.Fatalf("got %d triples, want 2: %+v", len(g.Triples), g.Triples)
	}
	// A trailing predicate-object list after the bracket is still accepted.
	doc2 := "[ a <http://ex/Work> ] <http://ex/p> \"y\" .\n"
	if g2, err := ParseTurtle([]byte(doc2)); err != nil || len(g2.Triples) != 2 {
		t.Fatalf("with trailing p-o list: err=%v triples=%d", err, len(g2.Triples))
	}
}
