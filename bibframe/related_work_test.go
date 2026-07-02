package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestNameTitleRelatedWork covers a 7xx name-title ($t present) becoming a related
// work instead of a spurious contribution (task 062): the contributor is gone and
// the related title (with the linking name as its creator) is preserved.
func TestNameTitleRelatedWork(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("700", '1', '2', codex.NewSubfield('a', "Austen, Jane,"),
			codex.NewSubfield('t', "Pride and prejudice")),
	))
	if len(g.Work.Contributions) != 0 {
		t.Errorf("name-title 7xx must not yield a Contribution; got %+v", g.Work.Contributions)
	}
	if len(g.Work.RelatedWorks) != 1 {
		t.Fatalf("want 1 related work; got %+v", g.Work.RelatedWorks)
	}
	rw := g.Work.RelatedWorks[0]
	if rw.Primary || rw.Class != "Person" || rw.Name != "Austen, Jane" || rw.Title.MainTitle != "Pride and prejudice" {
		t.Errorf("related work = %+v", rw)
	}
}

// TestPlainAddedEntryUnaffected confirms a 7xx with no $t still yields a normal
// contribution and no related work (task 062).
func TestPlainAddedEntryUnaffected(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Editor, An"), codex.NewSubfield('4', "edt")),
	))
	if len(g.Work.RelatedWorks) != 0 {
		t.Errorf("plain 7xx must not yield a related work; got %+v", g.Work.RelatedWorks)
	}
	if len(g.Work.Contributions) != 1 {
		t.Errorf("plain 7xx should yield one contribution; got %+v", g.Work.Contributions)
	}
}

// TestNameTitleRoundTrip confirms name-title access points survive Encode -> Decode
// back into 1xx/7xx $a/$t with the class-appropriate tag, and that the nested
// related work does not leak out as a second record (task 062).
func TestNameTitleRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Homer,"),
			codex.NewSubfield('t', "Odyssey")), // name-title main entry
		codex.NewDataField("710", '2', ' ', codex.NewSubfield('a', "Some Body"),
			codex.NewSubfield('t', "Annual report")), // corporate name-title added entry
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
		t.Fatalf("decoded %d records, want 1 (nested related work must not be its own record)", len(recs))
	}
	got := recs[0]
	if f := firstField(got, "100"); f == nil || f.SubfieldValue('a') != "Homer" || f.SubfieldValue('t') != "Odyssey" {
		t.Errorf("100 name-title not reconstructed; got %+v", f)
	}
	if f := firstField(got, "710"); f == nil || f.SubfieldValue('a') != "Some Body" || f.SubfieldValue('t') != "Annual report" {
		t.Errorf("710 name-title not reconstructed; got %+v", f)
	}
	// The 100$t main entry must not resurface as a contributor (no 700 either).
	if firstField(got, "700") != nil {
		t.Error("name-title must not reconstruct as a plain added entry")
	}
}
