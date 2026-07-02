package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// countFields returns every field of a record carrying the given tag.
func countFields(r *codex.Record, tag string) []codex.Field {
	var out []codex.Field
	for _, f := range r.Fields() {
		if f.Tag == tag {
			out = append(out, f)
		}
	}
	return out
}

// TestNoteFamilyForward covers the 5xx note routing: 500/504 to the Instance, 546
// to the Work, 505 to the Work's table of contents, each with its bf:noteType token
// (task 072).
func TestNoteFamilyForward(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("500", ' ', ' ', codex.NewSubfield('a', "General note.")),
		codex.NewDataField("504", ' ', ' ', codex.NewSubfield('a', "Includes bibliographical references.")),
		codex.NewDataField("546", ' ', ' ', codex.NewSubfield('a', "Text in English and French.")),
		codex.NewDataField("505", '0', ' ', codex.NewSubfield('a', "Ch. 1 -- Ch. 2 -- Ch. 3.")),
	))
	if len(g.Instance.Notes) != 2 {
		t.Fatalf("instance notes = %+v, want 2 (500 + 504)", g.Instance.Notes)
	}
	var sawGeneral, sawBib bool
	for _, n := range g.Instance.Notes {
		switch n.Type {
		case "":
			sawGeneral = n.Label == "General note."
		case "bibliography":
			sawBib = n.Label == "Includes bibliographical references."
		}
	}
	if !sawGeneral || !sawBib {
		t.Errorf("instance note types not routed: %+v", g.Instance.Notes)
	}
	if len(g.Work.Notes) != 1 || g.Work.Notes[0].Type != "language" {
		t.Errorf("work notes = %+v, want one language note", g.Work.Notes)
	}
	if len(g.Work.TableOfContents) != 1 || g.Work.TableOfContents[0] != "Ch. 1 -- Ch. 2 -- Ch. 3." {
		t.Errorf("work ToC = %+v, want one 505 entry", g.Work.TableOfContents)
	}
}

// TestNoteFamilyRoundTrip encodes a record carrying the whole 5xx note family and
// decodes it, asserting each note returns to its tag. Multiple Instance notes and
// multiple 505 entries exercise the JSON-LD array serialization that keeps repeated
// bf:note / bf:tableOfContents values from colliding on a single object key (task
// 072).
func TestNoteFamilyRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("500", ' ', ' ', codex.NewSubfield('a', "First general note.")),
		codex.NewDataField("500", ' ', ' ', codex.NewSubfield('a', "Second general note.")),
		codex.NewDataField("504", ' ', ' ', codex.NewSubfield('a', "Includes bibliographical references.")),
		codex.NewDataField("546", ' ', ' ', codex.NewSubfield('a', "In English and French.")),
		codex.NewDataField("505", '0', ' ', codex.NewSubfield('a', "Part one.")),
		codex.NewDataField("505", '0', ' ', codex.NewSubfield('a', "Part two.")),
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("decoded %d records, want 1", len(recs))
	}
	got := recs[0]
	if n := countFields(got, "500"); len(n) != 2 {
		t.Errorf("500 fields = %+v, want 2 (repeated notes must not collapse)", n)
	}
	if n := countFields(got, "504"); len(n) != 1 {
		t.Errorf("504 fields = %+v, want 1", n)
	}
	if n := countFields(got, "546"); len(n) != 1 {
		t.Errorf("546 fields = %+v, want 1", n)
	}
	if n := countFields(got, "505"); len(n) != 2 {
		t.Errorf("505 fields = %+v, want 2 (repeated ToC must not collapse)", n)
	}
}
