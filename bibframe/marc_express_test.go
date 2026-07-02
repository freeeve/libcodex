package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestMARCExpressConventions covers the OverDrive MARC Express read-path additions
// (task 057): 037 source-of-acquisition becomes an Instance identifier, 084 carries
// agency classifications (repeated $a with a $2 scheme), and a 650 _7 named-source
// subject is read rather than dropped.
func TestMARCExpressConventions(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "od12345")).
		AddField(codex.NewDataField("037", ' ', ' ',
			codex.NewSubfield('a', "ABC-123-RESERVE"),
			codex.NewSubfield('b', "OverDrive, Inc."))).
		AddField(codex.NewDataField("084", ' ', ' ',
			codex.NewSubfield('a', "FIC000000"),
			codex.NewSubfield('a', "FIC027000"),
			codex.NewSubfield('2', "bisacsh"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "A Title"))).
		AddField(codex.NewDataField("650", ' ', '7',
			codex.NewSubfield('a', "Fantasy"),
			codex.NewSubfield('2', "OverDrive")))
	g := FromRecord(rec)

	// 037 -> Instance identifier carrying the Reserve ID, source from $b.
	var reserve *Identifier
	for i := range g.Instance.Identifiers {
		if g.Instance.Identifiers[i].Value == "ABC-123-RESERVE" {
			reserve = &g.Instance.Identifiers[i]
		}
	}
	if reserve == nil {
		t.Fatalf("037 $a not read as an identifier; got %+v", g.Instance.Identifiers)
	}
	if reserve.Source != "OverDrive, Inc." {
		t.Errorf("037 identifier source = %q, want %q", reserve.Source, "OverDrive, Inc.")
	}

	// 084 -> two BISAC classifications with the $2 scheme as source.
	var bisac []Classification
	for _, c := range g.Work.Classifications {
		if c.Source == "bisacsh" {
			bisac = append(bisac, c)
		}
	}
	if len(bisac) != 2 {
		t.Fatalf("084 repeated $a -> %d bisacsh classifications, want 2 (%+v)", len(bisac), g.Work.Classifications)
	}
	if bisac[0].Value != "FIC000000" || bisac[1].Value != "FIC027000" {
		t.Errorf("084 classification values = %q, %q; want FIC000000, FIC027000", bisac[0].Value, bisac[1].Value)
	}

	// 650 _7 $2 OverDrive -> a bf:subject (indicator/$2 must not cause a skip).
	found := false
	for _, s := range g.Work.Subjects {
		if s.Class == "Topic" && s.Label == "Fantasy" {
			found = true
		}
	}
	if !found {
		t.Errorf("650 _7 subject not read as bf:subject; got %+v", g.Work.Subjects)
	}
}

// TestMARCExpressEmptyFieldsNoPanic guards the new read paths against fields that
// carry the tag but not the expected subfields.
func TestMARCExpressEmptyFieldsNoPanic(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("037", ' ', ' ', codex.NewSubfield('b', "Agency only"))).
		AddField(codex.NewDataField("084", ' ', ' ', codex.NewSubfield('2', "bisacsh"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))
	g := FromRecord(rec)
	for _, id := range g.Instance.Identifiers {
		if id.Value == "" {
			t.Errorf("empty 037 produced an identifier with no value: %+v", id)
		}
	}
	for _, c := range g.Work.Classifications {
		if c.Value == "" {
			t.Errorf("empty 084 produced a classification with no value: %+v", c)
		}
	}
}
