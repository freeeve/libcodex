package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestLinkingEntryForward covers 76x-78x linking entries -> bf:relation: the
// relationship code from the tag and (for 780/785) the second indicator, plus the
// linked resource's title, creator and ISSN (task 073).
func TestLinkingEntryForward(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("780", '0', '0', codex.NewSubfield('t', "Old title"),
			codex.NewSubfield('x', "1111-2222")), // continues
		codex.NewDataField("785", '0', '2', codex.NewSubfield('t', "New title"),
			codex.NewSubfield('a', "Publisher")), // supersededBy
		codex.NewDataField("773", '0', ' ', codex.NewSubfield('t', "Host journal")),  // partOf
		codex.NewDataField("776", '0', ' ', codex.NewSubfield('t', "Print version")), // otherPhysicalFormat
	))
	if len(g.Work.Relations) != 4 {
		t.Fatalf("relations = %+v, want 4", g.Work.Relations)
	}
	byCode := map[string]Relation{}
	for _, r := range g.Work.Relations {
		byCode[r.Relationship] = r
	}
	if r, ok := byCode["continues"]; !ok || r.Title != "Old title" || r.ISSN != "1111-2222" {
		t.Errorf("continues relation = %+v", r)
	}
	if r, ok := byCode["supersededBy"]; !ok || r.Title != "New title" || r.Name != "Publisher" {
		t.Errorf("supersededBy relation = %+v", r)
	}
	if _, ok := byCode["partOf"]; !ok {
		t.Errorf("missing partOf (773); got %+v", g.Work.Relations)
	}
	if _, ok := byCode["otherPhysicalFormat"]; !ok {
		t.Errorf("missing otherPhysicalFormat (776); got %+v", g.Work.Relations)
	}
}

// TestLinkingEntryRoundTrip encodes a record carrying preceding/succeeding/host/
// other-format links and decodes it, asserting each returns to its 76x-78x tag with
// the relationship-bearing second indicator and access-point subfields intact, and
// that no linked resource surfaces as its own record (task 073).
func TestLinkingEntryRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("780", '0', '2', codex.NewSubfield('a', "Prior Co."),
			codex.NewSubfield('t', "Predecessor"), codex.NewSubfield('x', "0000-1111")), // supersedes
		codex.NewDataField("785", '0', '0', codex.NewSubfield('t', "Successor")), // continuedBy
		codex.NewDataField("776", '0', ' ', codex.NewSubfield('t', "Online edition"),
			codex.NewSubfield('x', "2222-3333")), // otherPhysicalFormat
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
		t.Fatalf("decoded %d records, want 1 (linked resources must not be their own records)", len(recs))
	}
	got := recs[0]

	if f := firstField(got, "780"); f == nil || f.Ind2 != '2' ||
		f.SubfieldValue('a') != "Prior Co." || f.SubfieldValue('t') != "Predecessor" ||
		f.SubfieldValue('x') != "0000-1111" {
		t.Errorf("780 not reconstructed; got %+v", f)
	}
	if f := firstField(got, "785"); f == nil || f.Ind2 != '0' || f.SubfieldValue('t') != "Successor" {
		t.Errorf("785 not reconstructed; got %+v", f)
	}
	if f := firstField(got, "776"); f == nil || f.SubfieldValue('t') != "Online edition" ||
		f.SubfieldValue('x') != "2222-3333" {
		t.Errorf("776 not reconstructed; got %+v", f)
	}
}
