package rdf

import (
	"strings"
	"testing"
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
