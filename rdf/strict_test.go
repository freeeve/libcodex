package rdf

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

// truncatedDoc is the shape that motivated strictness: a well-formed statement
// followed by a line the writer never finished. Skipping it yields a smaller,
// well-formed graph, and no caller downstream can tell (libcat, task 115).
const truncatedDoc = "<http://w> <http://p> <http://o> <http://g> .\n<http://broken \n"

// wantSyntaxError asserts err is a *SyntaxError on the given line.
func wantSyntaxError(t *testing.T, err error, line int) {
	t.Helper()
	var se *SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v (%T), want *SyntaxError", err, err)
	}
	if se.Line != line {
		t.Errorf("SyntaxError.Line = %d, want %d", se.Line, line)
	}
	if !strings.Contains(se.Error(), "line "+strconv.Itoa(line)) {
		t.Errorf("Error() = %q, want it to name line %d", se.Error(), line)
	}
}

// TestParsersRejectTruncatedInput covers all four bulk entry points. A truncated
// document must not parse as a smaller graph.
func TestParsersRejectTruncatedInput(t *testing.T) {
	for name, parse := range map[string]func([]byte) error{
		"ParseNQuads":       func(b []byte) error { _, err := ParseNQuads(b); return err },
		"ParseNQuadsShared": func(b []byte) error { _, err := ParseNQuadsShared(b); return err },
		"ParseNTriples":     func(b []byte) error { _, err := ParseNTriples(b); return err },
		"ParseNTriplesShared": func(b []byte) error {
			_, err := ParseNTriplesShared(b)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			wantSyntaxError(t, parse([]byte(truncatedDoc)), 2)
		})
	}
}

// TestGarbageIsNotAnEmptyGraph pins the worst case from the report: a file that
// is not RDF at all used to parse as zero quads and no error.
func TestGarbageIsNotAnEmptyGraph(t *testing.T) {
	if _, err := ParseNQuads([]byte("this is not rdf at all\n")); err == nil {
		t.Error("ParseNQuads accepted a document containing no RDF")
	}
	if _, err := ParseNTriples([]byte("this is not rdf at all\n")); err == nil {
		t.Error("ParseNTriples accepted a document containing no RDF")
	}
}

// TestStreamingDecoderStrict checks io.EOF means end of input, not "end of input,
// and some of it was unreadable".
func TestStreamingDecoderStrict(t *testing.T) {
	d := NewDecoder(bytes.NewReader([]byte(truncatedDoc)), NQuads)
	if _, err := d.DecodeQuad(); err != nil {
		t.Fatalf("first quad: %v", err)
	}
	_, err := d.DecodeQuad()
	if errors.Is(err, io.EOF) {
		t.Fatal("decoder reported a clean EOF over a truncated line")
	}
	wantSyntaxError(t, err, 2)
}

// TestStreamingDecoderSkipMalformed checks the opt-in restores the old lenient
// behavior, ending in a genuine io.EOF.
func TestStreamingDecoderSkipMalformed(t *testing.T) {
	d := NewDecoder(bytes.NewReader([]byte(truncatedDoc)), NQuads).SkipMalformed(true)
	n := 0
	for {
		_, err := d.DecodeQuad()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("SkipMalformed decoder returned %v", err)
		}
		n++
	}
	if n != 1 {
		t.Errorf("decoded %d quads, want 1", n)
	}
}

// TestBlankAndCommentLinesAreNotErrors keeps the distinction the old bool
// collapsed: a line carrying no statement is not the same as a line that fails
// to parse.
func TestBlankAndCommentLinesAreNotErrors(t *testing.T) {
	doc := "# a comment\n\n   \n<http://a> <http://b> <http://c> .\n"
	g, err := ParseNTriples([]byte(doc))
	if err != nil {
		t.Fatalf("blank and comment lines must not be errors: %v", err)
	}
	if len(g.Triples) != 1 {
		t.Errorf("parsed %d triples, want 1", len(g.Triples))
	}
}

// TestSyntaxErrorNamesTheLateLine guards the line number on a document whose
// malformed line is not the second one, so an off-by-one cannot hide.
func TestSyntaxErrorNamesTheLateLine(t *testing.T) {
	var b strings.Builder
	for range 9 {
		b.WriteString("<http://a> <http://b> <http://c> .\n")
	}
	b.WriteString("<http://broken \n")
	wantSyntaxError(t, func() error { _, err := ParseNTriples([]byte(b.String())); return err }(), 10)
}

// TestSyntaxErrorLineIsRelativeToTheInput pins the hazard libcat hit (task 117):
// a bulk parser numbers from the start of the bytes it was given, so a caller
// chunking a large dump gets a plausible-looking wrong line. The streaming decoder
// reads across the chunks and numbers continuously, which is the answer.
func TestSyntaxErrorLineIsRelativeToTheInput(t *testing.T) {
	const good = "<http://a> <http://b> <http://c> .\n"
	chunk1 := strings.Repeat(good, 5)
	chunk2 := strings.Repeat(good, 2) + "<http://broken \n" // document line 8

	// Bulk, one chunk at a time: line 3 of chunk2, not line 8 of the document.
	_, err := ParseNTriples([]byte(chunk2))
	wantSyntaxError(t, err, 3)

	// Streamed across both chunks: the document's own line number.
	d := NewDecoder(io.MultiReader(strings.NewReader(chunk1), strings.NewReader(chunk2)), NTriples)
	for {
		_, err := d.Decode()
		if errors.Is(err, io.EOF) {
			t.Fatal("decoder reached EOF without reporting the malformed line")
		}
		if err != nil {
			wantSyntaxError(t, err, 8)
			return
		}
	}
}

// TestParseReturnsWhatItReadBeforeFailing documents that the partial graph is
// still returned alongside the error: a caller inspecting it must not mistake it
// for the whole document, which is exactly why the error exists.
func TestParseReturnsWhatItReadBeforeFailing(t *testing.T) {
	g, err := ParseNTriples([]byte(truncatedDoc))
	if err == nil {
		t.Fatal("want an error")
	}
	if len(g.Triples) != 1 {
		t.Errorf("partial graph has %d triples, want the 1 read before the bad line", len(g.Triples))
	}
}
