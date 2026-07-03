package rdf

import (
	"reflect"
	"runtime"
	"testing"
)

// sharedParseDoc exercises every term shape the line-based parsers produce:
// IRIs, blank nodes, plain / language-tagged / typed / escaped literals, named
// graphs, and the skipped comment and malformed lines.
const sharedParseDoc = `# comment
<http://example.org/s1> <http://example.org/p> "plain" .
<http://example.org/s1> <http://example.org/p> "français"@fr <http://example.org/g1> .
_:b1 <http://example.org/p> "42"^^<http://www.w3.org/2001/XMLSchema#integer> <http://example.org/g1> .
<http://example.org/s2> <http://example.org/p> _:b1 .
<http://example.org/s2> <http://example.org/p> "say \"hi\"\n" .
not a statement
`

// TestParseNQuadsShared checks the zero-copy variant yields exactly the dataset
// the copying parser does.
func TestParseNQuadsShared(t *testing.T) {
	want, err := ParseNQuads([]byte(sharedParseDoc))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseNQuadsShared([]byte(sharedParseDoc))
	if err != nil {
		t.Fatal(err)
	}
	if len(want.Quads) != 5 {
		t.Fatalf("parsed %d quads, want 5", len(want.Quads))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("shared parse diverges:\n got %+v\nwant %+v", got.Quads, want.Quads)
	}
}

// TestParseNTriplesShared checks the zero-copy variant yields exactly the graph
// the copying parser does.
func TestParseNTriplesShared(t *testing.T) {
	want, err := ParseNTriples([]byte(sharedParseDoc))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseNTriplesShared([]byte(sharedParseDoc))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("shared parse diverges:\n got %+v\nwant %+v", got.Triples, want.Triples)
	}
}

// TestParseNQuadsIndependentOfInput guards the copying parser's contract: the
// caller may scribble over data as soon as ParseNQuads returns.
func TestParseNQuadsIndependentOfInput(t *testing.T) {
	data := []byte(sharedParseDoc)
	d, err := ParseNQuads(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range data {
		data[i] = 'x'
	}
	if got := d.Quads[0].S.Value; got != "http://example.org/s1" {
		t.Fatalf("terms alias the caller's buffer: subject became %q", got)
	}
}

// TestParseNQuadsSharedSurvivesGC forces GC cycles after a shared parse and
// checks the buffer-backed terms are intact — the input []byte is kept alive
// only by the term strings pointing into it.
func TestParseNQuadsSharedSurvivesGC(t *testing.T) {
	d, err := ParseNQuadsShared([]byte(sharedParseDoc))
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		runtime.GC()
	}
	if got := d.Quads[0].O.Value; got != "plain" {
		t.Fatalf("literal corrupted after GC: %q", got)
	}
	if got := d.Quads[4].O.Value; got != "say \"hi\"\n" {
		t.Fatalf("arena-backed literal corrupted after GC: %q", got)
	}
}
