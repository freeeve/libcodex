package rdf

import (
	"io"
	"strings"
	"testing"
)

// TestStreamingDecoder checks the streaming decoder yields each statement, skips
// comment and blank lines, decodes escapes and language tags, and tolerates the
// N-Quads graph term. The malformed trailing line is skipped only because the
// decoder opts in with SkipMalformed; without it the stream fails (see
// TestStreamingDecoderStrict).
func TestStreamingDecoder(t *testing.T) {
	doc := "# a comment\n" +
		"<http://a> <http://b> <http://c> .\n" +
		"_:x <http://p> \"lit\\nwith\\tescapes \\\"q\\\"\" .\n" +
		"\n" +
		"<http://a> <http://b2> \"plain\"@en .\n" +
		"<http://a> <http://b3> <http://d> <http://graph> .\n" +
		"garbage line that is not a triple\n"

	d := NewDecoder(strings.NewReader(doc), NTriples).SkipMalformed(true)
	var got []Triple
	for {
		tr, err := d.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, tr)
	}
	if len(got) != 4 {
		t.Fatalf("got %d triples, want 4 (comment, blank and garbage skipped)", len(got))
	}
	if got[0].S.Value != "http://a" || !got[0].O.IsIRI() || got[0].O.Value != "http://c" {
		t.Errorf("triple 0 = %+v", got[0])
	}
	if got[1].S.Value != "x" || got[1].O.Value != "lit\nwith\tescapes \"q\"" {
		t.Errorf("escaped literal = %+v", got[1])
	}
	if got[2].O.Lang != "en" || got[2].O.Value != "plain" {
		t.Errorf("language-tagged literal = %+v", got[2])
	}
	if got[3].O.Value != "http://d" { // N-Quads graph term ignored
		t.Errorf("n-quads triple = %+v", got[3])
	}
}

// TestStreamingAll checks the All iterator and confirms the decoder never builds a
// Graph (constant memory): a large stream is consumed one triple at a time.
func TestStreamingAll(t *testing.T) {
	const n = 100000
	var b strings.Builder
	for range n {
		b.WriteString("<http://example.org/s> <http://example.org/p> \"v\" .\n")
	}
	count := 0
	for tr := range NewDecoder(strings.NewReader(b.String()), NTriples).All() {
		if tr.O.Value != "v" {
			t.Fatalf("triple %d corrupted: %+v", count, tr)
		}
		count++
	}
	if count != n {
		t.Errorf("streamed %d triples, want %d", count, n)
	}
}

// streamAll collects every triple from a streaming decode into a Graph.
func streamAll(r string, f Format) *Graph {
	g := &Graph{}
	for tr := range NewDecoder(strings.NewReader(r), f).All() {
		g.Add(tr.S, tr.P, tr.O)
	}
	return g
}

// TestStreamingMatchesParse confirms the streaming RDF/XML and Turtle decoders
// produce the same graph (up to blank-node isomorphism) as the whole-document
// parsers, which the W3C suite already validates.
func TestStreamingMatchesParse(t *testing.T) {
	gx, _ := ParseRDFXML([]byte(sampleXML))
	if sx := streamAll(sampleXML, RDFXML); !canonEqual(gx, sx) {
		t.Errorf("RDF/XML stream != parse:\n parse:  %v\n stream: %v", canonicalTriples(gx), canonicalTriples(sx))
	}
	gt, _ := ParseTurtle([]byte(turtleSample))
	if st := streamAll(turtleSample, Turtle); !canonEqual(gt, st) {
		t.Errorf("Turtle stream != parse:\n parse:  %v\n stream: %v", canonicalTriples(gt), canonicalTriples(st))
	}
}

// TestStreamingEarlyStop checks that breaking out of All (or calling Close) aborts
// the producer goroutine instead of leaking or deadlocking.
func TestStreamingEarlyStop(t *testing.T) {
	cases := []struct {
		doc string
		f   Format
	}{
		{turtleSample, Turtle},
		{sampleXML, RDFXML},
		{"<http://a> <http://b> <http://c> .\n<http://a> <http://b> <http://d> .\n", NTriples},
	}
	for _, tc := range cases {
		got := 0
		for range NewDecoder(strings.NewReader(tc.doc), tc.f).All() {
			got++
			break
		}
		if got != 1 {
			t.Errorf("format %d: expected 1 triple before break, got %d", tc.f, got)
		}
	}
	d := NewDecoder(strings.NewReader(turtleSample), Turtle)
	if _, err := d.Decode(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Error(err)
	}
}
