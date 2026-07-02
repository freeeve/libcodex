package rdf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// seedStream seeds a streaming fuzz target with valid documents of every format,
// edge cases, and the committed W3C corpus.
func seedStream(f *testing.F) {
	f.Add([]byte("<http://a> <http://b> <http://c> .\n"))
	f.Add([]byte("_:b <http://p> \"v\\n\\\"esc\\\"\"@en .\n<http://s> <http://p> <http://o> <http://g> .\n"))
	f.Add([]byte(turtleSample))
	f.Add([]byte(sampleXML))
	f.Add([]byte(sampleJSONLD))
	f.Add([]byte("@prefix ex: <http://e/> .\nex:s ex:p \"x\" , \"y\" ; ex:q [ ex:r ( 1 2.5 true ) ] .\n"))
	f.Add([]byte("@prefix : <#> .\n[] :x :y .\n"))
	// Statement-terminator edge cases from the review: a "." immediately followed
	// by a comment, and a "." right after a blank-node label (task 045).
	f.Add([]byte("<http://ex/s> <http://ex/p> <http://ex/o>.#comment\n"))
	f.Add([]byte("_:a <http://ex/p> _:y.\n"))
	for _, glob := range []string{"*.ttl", "*.nt"} {
		paths, _ := filepath.Glob(filepath.Join("testdata", "w3c", glob))
		for _, p := range paths {
			if b, err := os.ReadFile(p); err == nil {
				f.Add(b)
			}
		}
	}
}

// drain reads a decoder to exhaustion, returning every triple. It always reads to
// io.EOF (never breaks early) so the producer goroutine cannot leak.
func drain(d *Decoder) []Triple {
	var out []Triple
	for {
		tr, err := d.Decode()
		if err != nil {
			return out
		}
		out = append(out, tr)
	}
}

func sameTriples(t *testing.T, name string, stream, parse []Triple) {
	if len(stream) != len(parse) {
		t.Fatalf("%s: streamed %d triples, parsed %d", name, len(stream), len(parse))
	}
	for i := range stream {
		if stream[i] != parse[i] {
			t.Fatalf("%s: triple %d differs:\n stream %+v\n parse  %+v", name, i, stream[i], parse[i])
		}
	}
}

// FuzzStreamNTriples asserts the streaming N-Triples decoder yields exactly the
// triples the whole-document parser does (both run parseNTLine), and never panics.
// The N-Quads path is exercised too.
func FuzzStreamNTriples(f *testing.F) {
	seedStream(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		g, _ := ParseNTriples(data)
		sameTriples(t, "ntriples", drain(NewDecoder(bytes.NewReader(data), NTriples)), g.Triples)
		_ = drain(NewDecoder(bytes.NewReader(data), NQuads))
	})
}

// FuzzStreamNQuads asserts the streaming N-Quads decoder yields exactly the quads
// ParseNQuads does (both run parseNQuadLine), preserving graph terms, and never
// panics.
func FuzzStreamNQuads(f *testing.F) {
	seedStream(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		ds, _ := ParseNQuads(data)
		var streamed []Quad
		d := NewDecoder(bytes.NewReader(data), NQuads)
		for {
			q, err := d.DecodeQuad()
			if err != nil {
				break
			}
			streamed = append(streamed, q)
		}
		if len(streamed) != len(ds.Quads) {
			t.Fatalf("streamed %d quads, parsed %d", len(streamed), len(ds.Quads))
		}
		for i := range streamed {
			if streamed[i] != ds.Quads[i] {
				t.Fatalf("quad %d differs:\n stream %+v\n parse  %+v", i, streamed[i], ds.Quads[i])
			}
		}
	})
}

// FuzzNQuadsReSerialize parses N-Quads, serializes the dataset and reparses it,
// requiring the quad count to hold and the serialization to be idempotent, so no
// term (including the graph) is lost or corrupted across a round trip.
func FuzzNQuadsReSerialize(f *testing.F) {
	seedStream(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		ds, _ := ParseNQuads(data)
		if len(ds.Quads) == 0 {
			return
		}
		// The serializer emits clean UTF-8, so literals a lenient parse accepted with
		// invalid bytes are intentionally not byte-stable; skip them.
		for _, q := range ds.Quads {
			if q.O.IsLiteral() && !utf8.ValidString(q.O.Value) {
				return
			}
		}
		once := ds.NQuads()
		ds2, _ := ParseNQuads(once)
		if len(ds2.Quads) != len(ds.Quads) {
			t.Fatalf("re-serialize changed quad count: %d -> %d\n%s", len(ds.Quads), len(ds2.Quads), once)
		}
		if twice := ds2.NQuads(); !bytes.Equal(once, twice) {
			t.Fatalf("serialization not idempotent:\n once:  %q\n twice: %q", once, twice)
		}
	})
}

// FuzzStreamRDFXML asserts the streaming RDF/XML decoder yields exactly the same
// triples as ParseRDFXML (both run the same token walker), and never panics.
func FuzzStreamRDFXML(f *testing.F) {
	seedStream(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		g, _ := ParseRDFXML(data)
		sameTriples(t, "rdfxml", drain(NewDecoder(bytes.NewReader(data), RDFXML)), g.Triples)
	})
}

// FuzzStreamTurtle asserts the streaming Turtle decoder never panics or hangs.
// A triple-for-triple differential against ParseTurtle is not asserted over
// arbitrary input: the byte-level statement splitter cannot always frame a "."
// the same way the full grammar does (a "." before a letter is ambiguous between
// a statement boundary and a prefixed local name like "ex:a.b"), and the
// whole-document parser is lenient enough to emit empty-IRI triples for
// degenerate input. The specific "<o>.<s2>" terminator fix and the general valid
// case are covered by TestStreamTerminatorNoWhitespace and
// TestStreamingMatchesParse.
func FuzzStreamTurtle(f *testing.F) {
	seedStream(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = drain(NewDecoder(bytes.NewReader(data), Turtle))
	})
}

// FuzzSerializeRoundTrip parses arbitrary input, serializes the graph to N-Triples
// and to Turtle, and reparses each — requiring an isomorphic result, so the public
// serializers neither lose nor corrupt a triple.
func FuzzSerializeRoundTrip(f *testing.F) {
	seedStream(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		g, err := ParseNTriples(data)
		if err != nil || len(g.Triples) == 0 {
			return
		}
		// The serializers emit valid UTF-8, dropping invalid bytes a lenient parse
		// may have accepted in a literal — so round-trip is byte-stable only for
		// valid-UTF-8 literals. Skip the rest; this also isolates any real loss.
		for _, tr := range g.Triples {
			if tr.O.IsLiteral() && !utf8.ValidString(tr.O.Value) {
				return
			}
		}
		nt, err := ParseNTriples(g.NTriples())
		if err != nil {
			t.Fatalf("reparse N-Triples: %v", err)
		}
		if !canonEqual(g, nt) {
			t.Fatal("N-Triples round-trip changed the graph")
		}
		ttl := g.Turtle(nil)
		tt, err := ParseTurtle(ttl)
		if err != nil {
			t.Fatalf("reparse Turtle: %v\n%s", err, ttl)
		}
		if !canonEqual(g, tt) {
			t.Fatal("Turtle round-trip changed the graph")
		}
	})
}
