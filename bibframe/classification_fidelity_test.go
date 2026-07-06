package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// findClassification returns the first classification with the given portion value.
func findClassification(g *BIBFRAME, value string) *Classification {
	for i := range g.Work.Classifications {
		if g.Work.Classifications[i].Value == value {
			return &g.Work.Classifications[i]
		}
	}
	return nil
}

// TestClassificationItemPortion covers the 050/082/084 $b -> bf:itemPortion split
// and 082 $2 -> bf:source plus the ind1 Dewey edition (task 065).
func TestClassificationItemPortion(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("050", ' ', '0', codex.NewSubfield('a', "PS3556"), codex.NewSubfield('b', ".E446")),
		codex.NewDataField("082", '0', '0', codex.NewSubfield('a', "813.54"),
			codex.NewSubfield('b', "F32"), codex.NewSubfield('2', "23")),
		codex.NewDataField("084", ' ', ' ', codex.NewSubfield('a', "FIC000000"),
			codex.NewSubfield('b', "cutter"), codex.NewSubfield('2', "bisacsh")),
	))
	if c := findClassification(g, "PS3556"); c == nil || c.Class != "ClassificationLcc" || c.ItemPortion != ".E446" {
		t.Errorf("050 -> LCC portion+item; got %+v", c)
	}
	if c := findClassification(g, "813.54"); c == nil || c.Class != "ClassificationDdc" ||
		c.ItemPortion != "F32" || c.Source != "23" || c.Edition != "full" {
		t.Errorf("082 -> DDC portion+item+source+edition; got %+v", c)
	}
	if c := findClassification(g, "FIC000000"); c == nil || c.ItemPortion != "cutter" || c.Source != "bisacsh" {
		t.Errorf("084 -> classification portion+item+source; got %+v", c)
	}
}

// TestClassificationRoundTrip confirms the item portion, Dewey $2 and edition
// indicator survive Encode -> Decode (task 065).
func TestClassificationRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("050", ' ', '0', codex.NewSubfield('a', "PS3556"), codex.NewSubfield('b', ".E446")),
		codex.NewDataField("082", '1', '4', codex.NewSubfield('a', "813.54"),
			codex.NewSubfield('b', "F32"), codex.NewSubfield('2', "23")),
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	got := recs[0]
	if f := firstField(got, "050"); f == nil || f.SubfieldValue('a') != "PS3556" || f.SubfieldValue('b') != ".E446" {
		t.Errorf("050 round-trip $a/$b; got %+v", f)
	}
	f := firstField(got, "082")
	if f == nil || f.SubfieldValue('a') != "813.54" || f.SubfieldValue('b') != "F32" || f.SubfieldValue('2') != "23" {
		t.Errorf("082 round-trip $a/$b/$2; got %+v", f)
	}
	if f != nil && f.Ind1 != '1' { // abridged edition survives via ind1
		t.Errorf("082 ind1 = %c, want 1 (abridged)", f.Ind1)
	}
}

// classificationNode returns the generic bf:Classification node whose
// bf:classificationPortion equals value, or a zero Term if none matches.
func classificationNode(g *rdf.Graph, value string) rdf.Term {
	for _, s := range g.SubjectsOfType(bfNS + "Classification") {
		if v, ok := g.Literal(s, pClassPortion); ok && v == value {
			return s
		}
	}
	return rdf.Term{}
}

// TestClassificationLabelEmitted checks that a Classification.Label (the coded
// scheme's human display text, as the OverDrive/BISAC ingest supplies it) is hung
// on the classification node as rdfs:label, the display-only channel, while the
// coded Value stays in bf:classificationPortion (task 090).
func TestClassificationLabelEmitted(t *testing.T) {
	bib := &BIBFRAME{}
	bib.Work.Classifications = []Classification{{
		Class:  "Classification",
		Value:  "FIC000000",
		Label:  "FICTION / General",
		Source: "bisacsh",
	}}
	g := bib.Graph("od-123")
	node := classificationNode(g, "FIC000000")
	if node == (rdf.Term{}) {
		t.Fatalf("no classification node for FIC000000:\n%s", canonGraph(g))
	}
	if label, ok := g.Literal(node, pLabel); !ok || label != "FICTION / General" {
		t.Errorf("classification rdfs:label = %q (ok=%v), want %q", label, ok, "FICTION / General")
	}
}

// TestClassificationLabelOmittedWhenEmpty checks that a Classification without a
// Label emits no rdfs:label on its node, so the code alone survives.
func TestClassificationLabelOmittedWhenEmpty(t *testing.T) {
	bib := &BIBFRAME{}
	bib.Work.Classifications = []Classification{{Class: "Classification", Value: "FIC000000", Source: "bisacsh"}}
	g := bib.Graph("od-123")
	node := classificationNode(g, "FIC000000")
	if node == (rdf.Term{}) {
		t.Fatalf("no classification node for FIC000000:\n%s", canonGraph(g))
	}
	if label, ok := g.Literal(node, pLabel); ok {
		t.Errorf("unlabeled classification emitted rdfs:label = %q, want none", label)
	}
}

// TestClassificationLabelLostThroughMARC documents that Label has no standard MARC
// channel: a Classification round-tripped through the graph -> MARC crosswalk keeps
// its code ($a) but loses the label.
func TestClassificationLabelLostThroughMARC(t *testing.T) {
	bib := &BIBFRAME{}
	bib.Work.Classifications = []Classification{{
		Class:  "Classification",
		Value:  "FIC000000",
		Label:  "FICTION / General",
		Source: "bisacsh",
	}}
	nt := bib.Graph("od-123").NTriples()
	recs, err := Decode(nt)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f := firstField(recs[0], "072")
	if f == nil || f.SubfieldValue('a') != "FIC000000" {
		t.Fatalf("072 $a = code; got %+v", f)
	}
	for _, sub := range f.Subfields {
		if sub.Value == "FICTION / General" {
			t.Errorf("label leaked into 072 as $%c; no MARC channel is expected", sub.Code)
		}
	}
}
