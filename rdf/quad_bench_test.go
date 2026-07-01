package rdf

import (
	"strconv"
	"strings"
	"testing"
)

// ntDoc builds an n-line N-Triples document.
func ntDoc(n int) string {
	var b strings.Builder
	for i := range n {
		id := strconv.Itoa(i)
		b.WriteString("<http://example.org/s" + id + "> <http://example.org/p> \"value " + id + "\" .\n")
	}
	return b.String()
}

// nqDoc builds an n-line N-Quads document spread across 8 named graphs.
func nqDoc(n int) string {
	var b strings.Builder
	for i := range n {
		id := strconv.Itoa(i)
		b.WriteString("<http://example.org/s" + id + "> <http://example.org/p> \"value " + id +
			"\" <http://example.org/g" + strconv.Itoa(i%8) + "> .\n")
	}
	return b.String()
}

func BenchmarkParseNTriples(b *testing.B) {
	data := []byte(ntDoc(5000))
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if g, _ := ParseNTriples(data); len(g.Triples) != 5000 {
			b.Fatalf("got %d triples", len(g.Triples))
		}
	}
}

func BenchmarkParseNQuads(b *testing.B) {
	data := []byte(nqDoc(5000))
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if d, _ := ParseNQuads(data); len(d.Quads) != 5000 {
			b.Fatalf("got %d quads", len(d.Quads))
		}
	}
}

func BenchmarkGraphNQuads(b *testing.B) {
	g, _ := ParseNTriples([]byte(ntDoc(5000)))
	graph := NewIRI("http://example.org/provenance")
	b.ReportAllocs()
	for b.Loop() {
		_ = g.NQuads(graph)
	}
}

func BenchmarkDatasetNQuads(b *testing.B) {
	d, _ := ParseNQuads([]byte(nqDoc(5000)))
	b.ReportAllocs()
	for b.Loop() {
		_ = d.NQuads()
	}
}

// BenchmarkStreamTurtle streams a Turtle document through the decoder, exercising
// the statement splitter that reads into its buffer's spare capacity rather than
// allocating a fresh chunk per read.
func BenchmarkStreamTurtle(b *testing.B) {
	data := ntDoc(5000) // N-Triples is valid Turtle
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		d := NewDecoder(strings.NewReader(data), Turtle)
		n := 0
		for {
			if _, err := d.Decode(); err != nil {
				break
			}
			n++
		}
		if n != 5000 {
			b.Fatalf("got %d triples", n)
		}
	}
}
