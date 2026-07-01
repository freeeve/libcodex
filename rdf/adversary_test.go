package rdf

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDeepNestingDoesNotOverflow checks that adversarial deeply nested input is
// rejected with an error rather than recursing until the goroutine stack
// overflows — a fatal crash that recover cannot catch. The depths here exceed
// maxParseDepth but, without the guard, are still shallow enough to recurse
// (the unguarded overflow needs hundreds of thousands of levels), so the test
// proves the guard fires, not merely that a crash is avoided.
func TestDeepNestingDoesNotOverflow(t *testing.T) {
	const n = maxParseDepth + 5000

	t.Run("turtle blank property lists", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("<u:s> <u:p> ")
		b.WriteString(strings.Repeat("[ <u:p> ", n))
		b.WriteString("1")
		b.WriteString(strings.Repeat(" ]", n))
		b.WriteString(" .")
		if _, err := ParseTurtle([]byte(b.String())); err == nil {
			t.Fatal("deeply nested [ … ] parsed without error; depth guard missing")
		}
	})

	t.Run("turtle collections", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("<u:s> <u:p> ")
		b.WriteString(strings.Repeat("(", n))
		b.WriteString(strings.Repeat(")", n))
		b.WriteString(" .")
		if _, err := ParseTurtle([]byte(b.String())); err == nil {
			t.Fatal("deeply nested ( … ) parsed without error; depth guard missing")
		}
	})

	t.Run("rdfxml elements", func(t *testing.T) {
		var b strings.Builder
		b.WriteString(`<rdf:RDF xmlns:rdf="` + NS + `" xmlns:e="u:"><rdf:Description rdf:about="u:s">`)
		b.WriteString(strings.Repeat(`<e:p><rdf:Description>`, n))
		b.WriteString(strings.Repeat(`</rdf:Description></e:p>`, n))
		b.WriteString(`</rdf:Description></rdf:RDF>`)
		if _, err := ParseRDFXML([]byte(b.String())); err == nil {
			t.Fatal("deeply nested RDF/XML parsed without error; depth guard missing")
		}
	})
}

// TestModerateNestingStillParses confirms the depth guard does not reject the
// shallow nesting real documents use.
func TestModerateNestingStillParses(t *testing.T) {
	var b strings.Builder
	b.WriteString("<u:s> <u:p> ")
	b.WriteString(strings.Repeat("[ <u:p> ", 100))
	b.WriteString("1")
	b.WriteString(strings.Repeat(" ]", 100))
	b.WriteString(" .")
	if _, err := ParseTurtle([]byte(b.String())); err != nil {
		t.Fatalf("100-deep nesting should parse, got %v", err)
	}
}

// TestStreamingStatementBounded checks that a streaming Turtle statement that
// never terminates is rejected once it exceeds maxStatementBytes, instead of
// buffering the whole input and re-scanning it quadratically.
func TestStreamingStatementBounded(t *testing.T) {
	// One predicate-object list joined by ';' with no '.', larger than the cap.
	unit := "<http://s> <http://p> <http://o> ; "
	reps := (maxStatementBytes / len(unit)) + 1000
	doc := strings.Repeat(unit, reps)

	d := NewDecoder(strings.NewReader(doc), Turtle)
	var err error
	for {
		if _, e := d.Decode(); e != nil {
			err = e
			break
		}
	}
	if err != errStatementTooLarge {
		t.Fatalf("oversized statement: got err %v, want errStatementTooLarge", err)
	}
}

// TestCanonGadgetGraphMemoized checks that the first-degree-hash memoization keeps
// a graph of many symmetric 2-node gadgets cheap. Every gadget's blank nodes share
// one first-degree hash and so drive n-degree hashing; without memoization the
// recursion re-serializes each node's quads over and over, burning enormous CPU
// while staying under the permutation budget. It must finish promptly (canonicalize
// or fail fast with ErrCanonComplexity), never hang.
func TestCanonGadgetGraphMemoized(t *testing.T) {
	const m, e = 200, 500
	var b strings.Builder
	for k := range m {
		a := "_:g" + strconv.Itoa(k) + "a"
		bb := "_:g" + strconv.Itoa(k) + "b"
		b.WriteString(a + " <u:p> " + bb + " .\n")
		b.WriteString(bb + " <u:p> " + a + " .\n")
		for i := range e {
			p := " <u:q" + strconv.Itoa(i) + "> "
			b.WriteString(a + p + "\"x\" .\n")
			b.WriteString(bb + p + "\"x\" .\n")
		}
	}
	ds, _ := ParseNQuads([]byte(b.String()))

	done := make(chan error, 1)
	go func() {
		_, err := ds.Canonical()
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil && err != ErrCanonComplexity {
			t.Fatalf("gadget graph: unexpected error %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("gadget graph did not canonicalize within 30s; first-degree memoization missing")
	}
}

// TestStreamingRDFXMLLiteralBounded checks that an RDF/XML literal accumulated
// across many CharData tokens is rejected once it exceeds maxLiteralBytes, rather
// than buffering without bound. Comments split the text into separate CharData
// tokens so parseProperty accumulates them, exercising the per-property cap.
func TestStreamingRDFXMLLiteralBounded(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`<rdf:RDF xmlns:rdf="` + NS + `" xmlns:e="u:"><rdf:Description rdf:about="u:s"><e:p>`)
	chunk := strings.Repeat("a", 60000) + "<!--c-->"
	for buf.Len() < maxLiteralBytes+100000 {
		buf.WriteString(chunk)
	}

	d := NewDecoder(bytes.NewReader(buf.Bytes()), RDFXML)
	var err error
	for {
		if _, e := d.Decode(); e != nil {
			err = e
			break
		}
	}
	if err != errLiteralTooLarge {
		t.Fatalf("oversized RDF/XML literal: got err %v, want errLiteralTooLarge", err)
	}
}
