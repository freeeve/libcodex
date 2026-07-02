package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
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
