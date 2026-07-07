package skos

import (
	"strings"
	"testing"
)

// fixture is a two-concept scheme: a1 (multilingual, with an altLabel and a
// broader link to b1) and b1.
const fixture = `<https://ex.org/c/a1> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://www.w3.org/2004/02/skos/core#Concept> .
<https://ex.org/c/a1> <http://purl.org/dc/terms/identifier> "a1" .
<https://ex.org/c/a1> <http://www.w3.org/2004/02/skos/core#prefLabel> "Alfa"@es .
<https://ex.org/c/a1> <http://www.w3.org/2004/02/skos/core#prefLabel> "Alpha"@en .
<https://ex.org/c/a1> <http://www.w3.org/2004/02/skos/core#altLabel> "A-one"@en .
<https://ex.org/c/a1> <http://www.w3.org/2004/02/skos/core#broader> <https://ex.org/c/b1> .
<https://ex.org/c/a1> <http://www.w3.org/2004/02/skos/core#inScheme> <https://ex.org/scheme> .
<https://ex.org/c/a1> <http://www.w3.org/2000/01/rdf-schema#comment> "A definition." .
<https://ex.org/c/b1> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://www.w3.org/2004/02/skos/core#Concept> .
<https://ex.org/c/b1> <http://purl.org/dc/terms/identifier> "b1" .
<https://ex.org/c/b1> <http://www.w3.org/2004/02/skos/core#prefLabel> "Beta"@en .
`

func TestParse(t *testing.T) {
	cs, err := Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("concepts = %d, want 2", len(cs))
	}
	// Sorted by id: a1, b1.
	a := cs[0]
	if a.ID != "a1" {
		t.Errorf("id = %q, want a1", a.ID)
	}
	if a.PrefLabel() != "Alpha" { // English-preferred over the Spanish prefLabel
		t.Errorf("PrefLabel = %q, want Alpha", a.PrefLabel())
	}
	if a.Scheme != "https://ex.org/scheme" {
		t.Errorf("scheme = %q", a.Scheme)
	}
	if len(a.Alt) != 1 || a.Alt[0].Text != "A-one" || a.Alt[0].Lang != "en" {
		t.Errorf("alt = %+v", a.Alt)
	}
	if len(a.Broader) != 1 || a.Broader[0].ID != "b1" || a.Broader[0].Label != "Beta" {
		t.Errorf("broader not resolved to b1/Beta: %+v", a.Broader)
	}
	if len(a.Notes) != 1 || a.Notes[0].Text != "A definition." {
		t.Errorf("notes = %+v", a.Notes)
	}
}

// TestParseIDFallback checks a concept with no dc:identifier takes the IRI's last
// path segment as its id.
func TestParseIDFallback(t *testing.T) {
	nt := `<https://ex.org/c/x9> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://www.w3.org/2004/02/skos/core#Concept> .
<https://ex.org/c/x9> <http://www.w3.org/2004/02/skos/core#prefLabel> "Nine"@en .
`
	cs, err := Parse([]byte(nt))
	if err != nil || len(cs) != 1 {
		t.Fatalf("Parse: %v (%d concepts)", err, len(cs))
	}
	if cs[0].ID != "x9" {
		t.Errorf("id fallback = %q, want x9", cs[0].ID)
	}
}

func TestRecordCrosswalk(t *testing.T) {
	cs, _ := Parse([]byte(fixture))
	rec := cs[0].Record() // a1

	if rec.Leader().RecordType() != 'z' {
		t.Errorf("leader/06 = %c, want z (authority)", rec.Leader().RecordType())
	}
	if got := rec.ControlField("001"); got != "a1" {
		t.Errorf("001 = %q, want a1", got)
	}
	if got := rec.SubfieldValue("024", 'a'); got != "https://ex.org/c/a1" {
		t.Errorf("024 $a = %q", got)
	}
	if got := rec.SubfieldValue("024", '2'); got != "uri" {
		t.Errorf("024 $2 = %q, want uri", got)
	}
	if got := rec.SubfieldValue("150", 'a'); got != "Alpha" {
		t.Errorf("150 $a = %q, want Alpha", got)
	}
	// Two 450s: the Spanish prefLabel and the English altLabel, both with $9 lang.
	if got := rec.SubfieldValues("450", 'a'); len(got) != 2 {
		t.Errorf("450 $a values = %v, want 2 (Alfa, A-one)", got)
	}
	f, ok := rec.DataField("550")
	if !ok {
		t.Fatal("missing 550 broader tracing")
	}
	if f.SubfieldValue('w') != "g" || f.SubfieldValue('a') != "Beta" || f.SubfieldValue('0') != "https://ex.org/c/b1" {
		t.Errorf("550 = %+v, want $wg $aBeta $0<b1 iri>", f)
	}
	if got := rec.SubfieldValue("680", 'i'); got != "A definition." {
		t.Errorf("680 $i = %q", got)
	}
}

// TestParseTurtle checks the serialization is autodetected: the same scheme in
// Turtle parses to the same concepts.
func TestParseTurtle(t *testing.T) {
	ttl := `@prefix skos: <http://www.w3.org/2004/02/skos/core#> .
@prefix dct: <http://purl.org/dc/terms/> .
<https://ex.org/c/a1> a skos:Concept ; dct:identifier "a1" ; skos:prefLabel "Alpha"@en .
`
	cs, err := Parse([]byte(ttl))
	if err != nil {
		t.Fatalf("Parse turtle: %v", err)
	}
	if len(cs) != 1 || cs[0].PrefLabel() != "Alpha" {
		t.Errorf("turtle parse = %+v", cs)
	}
}

func TestReadEmpty(t *testing.T) {
	cs, err := Read(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 0 {
		t.Errorf("empty input = %d concepts, want 0", len(cs))
	}
}
